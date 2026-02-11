'use client';

/**
 * Template management — admin page (read-only list for V1).
 *
 * OpenAPI: GET /templates
 * master-flow Stage 3, Step 3: Template defines OS image + cloud-init.
 * ADR-0018: Templates stored in DB, define OS image source and cloud-init only.
 *
 * Search best practices (Context7 / React docs / Ant Design v5):
 * - useDeferredValue: React-recommended debounce for search input
 * - filterDropdown: Ant Design column-level search with highlight
 * - filters + onFilter: Ant Design column-level enum filter for OS Family / Status
 * - useMemo: cache filtered results to avoid re-render on unchanged data
 */
import { useState, useDeferredValue, useMemo, useRef } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Tag,
    Card,
    Input,
    Empty,
} from 'antd';
import type { InputRef } from 'antd';
import type { ColumnsType, FilterDropdownProps } from 'antd/es/table/interface';
import {
    ReloadOutlined,
    FileTextOutlined,
    SearchOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type Template = components['schemas']['Template'];
type TemplateList = components['schemas']['TemplateList'];

const OS_COLOR_MAP: Record<string, string> = {
    linux: 'green',
    windows: 'blue',
    centos: 'orange',
    ubuntu: 'geekblue',
    debian: 'purple',
    rhel: 'red',
    fedora: 'cyan',
};

/**
 * Highlight matching text within a string.
 * Used for visual feedback in filterDropdown search results.
 */
function highlightText(text: string, highlight: string): React.ReactNode {
    if (!highlight) return text;
    const index = text.toLowerCase().indexOf(highlight.toLowerCase());
    if (index === -1) return text;
    const before = text.slice(0, index);
    const match = text.slice(index, index + highlight.length);
    const after = text.slice(index + highlight.length);
    return (
        <>
            {before}
            <span style={{ backgroundColor: '#ffc069', fontWeight: 600, padding: '0 2px' }}>
                {match}
            </span>
            {after}
        </>
    );
}

export default function TemplatesPage() {
    const { t } = useTranslation(['admin', 'common']);
    const [page, setPage] = useState(1);
    const [globalSearch, setGlobalSearch] = useState('');
    const deferredSearch = useDeferredValue(globalSearch);
    const isStale = globalSearch !== deferredSearch;
    const searchInputRef = useRef<InputRef>(null);

    // Column-level search state (for filterDropdown highlight)
    const [searchedColumn, setSearchedColumn] = useState('');
    const [searchText, setSearchText] = useState('');

    // Fetch templates
    const { data, isLoading, refetch } = useApiGet<TemplateList>(
        ['admin-templates', page],
        () => api.GET('/templates', {
            params: { query: { page, per_page: 20 } },
        })
    );

    // Derive unique OS families for column filters
    const osFamilyFilters = useMemo(() => {
        const families = new Set<string>();
        (data?.items ?? []).forEach((tpl) => {
            if (tpl.os_family) families.add(tpl.os_family);
        });
        return Array.from(families).sort().map((f) => ({ text: f, value: f }));
    }, [data?.items]);

    // useMemo: cache filtered results based on deferred search value
    const filteredItems = useMemo(() => {
        const items = data?.items ?? [];
        if (!deferredSearch) return items;
        const q = deferredSearch.toLowerCase();
        return items.filter((tpl) =>
            tpl.name.toLowerCase().includes(q) ||
            (tpl.display_name ?? '').toLowerCase().includes(q) ||
            (tpl.description ?? '').toLowerCase().includes(q) ||
            (tpl.os_family ?? '').toLowerCase().includes(q)
        );
    }, [data?.items, deferredSearch]);

    /**
     * Ant Design best practice: filterDropdown for free-text column search.
     * Supports Enter to confirm + Reset to clear.
     */
    const getColumnSearchProps = (dataIndex: keyof Template): Partial<ColumnsType<Template>[number]> => ({
        filterDropdown: ({ setSelectedKeys, selectedKeys, confirm, clearFilters }: FilterDropdownProps) => (
            <div style={{ padding: 8 }} onKeyDown={(e) => e.stopPropagation()}>
                <Input
                    ref={searchInputRef}
                    placeholder={`${t('common:button.search')} ${dataIndex}`}
                    value={selectedKeys[0]}
                    onChange={(e) => setSelectedKeys(e.target.value ? [e.target.value] : [])}
                    onPressEnter={() => {
                        confirm();
                        setSearchText(selectedKeys[0] as string);
                        setSearchedColumn(dataIndex);
                    }}
                    style={{ marginBottom: 8, display: 'block' }}
                />
                <Space>
                    <Button
                        type="primary"
                        onClick={() => {
                            confirm();
                            setSearchText(selectedKeys[0] as string);
                            setSearchedColumn(dataIndex);
                        }}
                        icon={<SearchOutlined />}
                        size="small"
                        style={{ width: 90 }}
                    >
                        {t('common:button.search')}
                    </Button>
                    <Button
                        onClick={() => {
                            clearFilters?.();
                            setSearchText('');
                            setSearchedColumn('');
                            confirm();
                        }}
                        size="small"
                        style={{ width: 90 }}
                    >
                        {t('common:button.reset')}
                    </Button>
                </Space>
            </div>
        ),
        filterIcon: (filtered: boolean) => (
            <SearchOutlined style={{ color: filtered ? '#1677ff' : undefined }} />
        ),
        onFilter: (value, record) =>
            (record[dataIndex] ?? '').toString().toLowerCase().includes((value as string).toLowerCase()),
        onFilterDropdownOpenChange: (visible) => {
            if (visible) {
                setTimeout(() => searchInputRef.current?.select(), 100);
            }
        },
    });

    const columns: ColumnsType<Template> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            ...getColumnSearchProps('name'),
            sorter: (a, b) => a.name.localeCompare(b.name),
            render: (name: string, record: Template) => (
                <Space>
                    <FileTextOutlined style={{ color: '#1677ff' }} />
                    <div>
                        <Text strong>
                            {searchedColumn === 'name'
                                ? highlightText(record.display_name ?? name, searchText)
                                : (record.display_name ?? name)}
                        </Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {searchedColumn === 'name' ? highlightText(name, searchText) : name}
                        </Text>
                    </div>
                </Space>
            ),
        },
        {
            title: t('templates.os_family'),
            dataIndex: 'os_family',
            key: 'os_family',
            width: 120,
            // Ant Design best practice: enum filters with onFilter
            filters: osFamilyFilters,
            onFilter: (value, record) => record.os_family === value,
            render: (family: string | undefined) => {
                if (!family) return <Text type="secondary">—</Text>;
                const color = OS_COLOR_MAP[family.toLowerCase()] ?? 'default';
                return <Tag color={color}>{family}</Tag>;
            },
        },
        {
            title: t('templates.os_version'),
            dataIndex: 'os_version',
            key: 'os_version',
            width: 120,
            render: (v: string | undefined) => v ? <Tag>{v}</Tag> : '—',
        },
        {
            title: t('templates.version'),
            dataIndex: 'version',
            key: 'version',
            width: 90,
            align: 'center' as const,
            sorter: (a, b) => a.version - b.version,
            render: (v: number) => (
                <Tag color="processing">v{v}</Tag>
            ),
        },
        {
            title: t('templates.enabled'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 90,
            // Ant Design best practice: boolean enum filter
            filters: [
                { text: 'Active', value: true },
                { text: 'Disabled', value: false },
            ],
            onFilter: (value, record) => (record.enabled !== false) === value,
            render: (enabled: boolean | undefined) => (
                <Tag color={enabled !== false ? 'green' : 'default'}>
                    {enabled !== false ? 'Active' : 'Disabled'}
                </Tag>
            ),
        },
        {
            title: t('common:table.description'),
            dataIndex: 'description',
            key: 'description',
            ellipsis: true,
            render: (desc: string | undefined) => (
                <Text type="secondary">{desc || '—'}</Text>
            ),
        },
    ];

    return (
        <div>
            <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 24,
            }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('templates.title')}</Title>
                    <Text type="secondary">{t('templates.subtitle')}</Text>
                </div>
                <Space>
                    <Input
                        placeholder={t('common:button.search')}
                        prefix={<SearchOutlined />}
                        value={globalSearch}
                        onChange={(e) => setGlobalSearch(e.target.value)}
                        allowClear
                        style={{ width: 220 }}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                {/* useDeferredValue: opacity feedback during deferred update */}
                <div style={{
                    opacity: isStale ? 0.6 : 1,
                    transition: isStale ? 'opacity 0.2s 0.1s linear' : 'opacity 0s 0s linear',
                }}>
                    <Table<Template>
                        columns={columns}
                        dataSource={filteredItems}
                        rowKey="id"
                        loading={isLoading}
                        pagination={{
                            current: page,
                            total: data?.pagination?.total ?? filteredItems.length,
                            pageSize: 20,
                            onChange: setPage,
                            showTotal: (total) => t('common:table.total', { total }),
                            showSizeChanger: false,
                        }}
                        size="middle"
                        locale={{
                            emptyText: (
                                <Empty
                                    description={
                                        deferredSearch
                                            ? t('common:message.no_data')
                                            : t('templates.empty')
                                    }
                                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                                />
                            ),
                        }}
                    />
                </div>
            </Card>
        </div>
    );
}

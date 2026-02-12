'use client';

/**
 * InstanceSize management — admin page (read-only list for V1).
 *
 * OpenAPI: GET /instance-sizes
 * master-flow Stage 3, Step 4: InstanceSize defines hardware resources.
 * ADR-0018: Hardware capability requirements (GPU/SR-IOV/Hugepages) in InstanceSize.
 *
 * Search best practices (Context7 / React docs / Ant Design v5):
 * - useDeferredValue: React-recommended debounce for search input
 * - filterDropdown: Ant Design column-level search with highlight
 * - filters + onFilter: Ant Design column-level enum filter for capabilities / status
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
    HddOutlined,
    SearchOutlined,
    ThunderboltOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type InstanceSize = components['schemas']['InstanceSize'];
type InstanceSizeList = components['schemas']['InstanceSizeList'];

/**
 * Format memory from MB to human-readable (GB if >= 1024).
 */
function formatMemory(mb: number): string {
    if (mb >= 1024) {
        const gb = mb / 1024;
        return `${Number.isInteger(gb) ? gb : gb.toFixed(1)} GB`;
    }
    return `${mb} MB`;
}

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

/**
 * Derive capability tags for a given InstanceSize record.
 */
function getCapabilityLabels(record: InstanceSize): string[] {
    const caps: string[] = [];
    if (record.requires_gpu) caps.push('GPU');
    if (record.requires_sriov) caps.push('SR-IOV');
    if (record.requires_hugepages) caps.push('Hugepages');
    if (record.dedicated_cpu) caps.push('Dedicated CPU');
    return caps;
}

export default function InstanceSizesPage() {
    const { t } = useTranslation(['admin', 'common']);
    const [globalSearch, setGlobalSearch] = useState('');
    const deferredSearch = useDeferredValue(globalSearch);
    const isStale = globalSearch !== deferredSearch;
    const searchInputRef = useRef<InputRef>(null);

    // Column-level search state (for filterDropdown highlight)
    const [searchedColumn, setSearchedColumn] = useState('');
    const [searchText, setSearchText] = useState('');

    // Fetch instance sizes
    const { data, isLoading, refetch } = useApiGet<InstanceSizeList>(
        ['admin-instance-sizes'],
        () => api.GET('/instance-sizes')
    );

    // useMemo: cache filtered results based on deferred search value
    const filteredItems = useMemo(() => {
        const items = data?.items ?? [];
        if (!deferredSearch) return items;
        const q = deferredSearch.toLowerCase();
        return items.filter((sz) =>
            sz.name.toLowerCase().includes(q) ||
            (sz.display_name ?? '').toLowerCase().includes(q) ||
            (sz.description ?? '').toLowerCase().includes(q) ||
            getCapabilityLabels(sz).some((c) => c.toLowerCase().includes(q))
        );
    }, [data?.items, deferredSearch]);

    /**
     * Ant Design best practice: filterDropdown for free-text column search.
     */
    const getColumnSearchProps = (dataIndex: keyof InstanceSize): Partial<ColumnsType<InstanceSize>[number]> => ({
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
        filterDropdownProps: {
            onOpenChange: (visible) => {
                if (visible) {
                    setTimeout(() => searchInputRef.current?.select(), 100);
                }
            },
        },
    });

    const columns: ColumnsType<InstanceSize> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            ...getColumnSearchProps('name'),
            sorter: (a, b) => a.name.localeCompare(b.name),
            render: (name: string, record: InstanceSize) => (
                <Space>
                    <HddOutlined style={{ color: '#1677ff' }} />
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
            title: t('instanceSizes.cpu'),
            dataIndex: 'cpu_cores',
            key: 'cpu_cores',
            width: 100,
            align: 'center' as const,
            sorter: (a, b) => a.cpu_cores - b.cpu_cores,
            render: (cores: number, record: InstanceSize) => (
                <Space direction="vertical" size={0} style={{ textAlign: 'center' }}>
                    <Text strong>{cores} {t('instanceSizes.cores')}</Text>
                    {record.dedicated_cpu && (
                        <Tag color="orange" style={{ fontSize: 10 }}>
                            <ThunderboltOutlined /> {t('instanceSizes.dedicated')}
                        </Tag>
                    )}
                </Space>
            ),
        },
        {
            title: t('instanceSizes.memory'),
            dataIndex: 'memory_mb',
            key: 'memory_mb',
            width: 100,
            align: 'center' as const,
            sorter: (a, b) => a.memory_mb - b.memory_mb,
            render: (mb: number) => <Text strong>{formatMemory(mb)}</Text>,
        },
        {
            title: t('instanceSizes.disk'),
            dataIndex: 'disk_gb',
            key: 'disk_gb',
            width: 100,
            align: 'center' as const,
            sorter: (a, b) => (a.disk_gb ?? 0) - (b.disk_gb ?? 0),
            render: (gb: number | undefined) => gb ? <Text>{gb} GB</Text> : <Text type="secondary">—</Text>,
        },
        {
            title: t('instanceSizes.capabilities'),
            key: 'capabilities',
            width: 220,
            // Ant Design best practice: multi-select enum column filter
            filters: [
                { text: 'GPU', value: 'gpu' },
                { text: 'SR-IOV', value: 'sriov' },
                { text: 'Hugepages', value: 'hugepages' },
                { text: 'Dedicated CPU', value: 'dedicated' },
            ],
            onFilter: (value, record) => {
                switch (value) {
                    case 'gpu': return !!record.requires_gpu;
                    case 'sriov': return !!record.requires_sriov;
                    case 'hugepages': return !!record.requires_hugepages;
                    case 'dedicated': return !!record.dedicated_cpu;
                    default: return false;
                }
            },
            filterMultiple: true,
            render: (_: unknown, record: InstanceSize) => {
                const tags: React.ReactNode[] = [];
                if (record.requires_gpu) {
                    tags.push(<Tag key="gpu" color="volcano">GPU</Tag>);
                }
                if (record.requires_sriov) {
                    tags.push(<Tag key="sriov" color="purple">SR-IOV</Tag>);
                }
                if (record.requires_hugepages) {
                    tags.push(
                        <Tag key="hugepages" color="cyan">
                            Hugepages {record.hugepages_size ? `(${record.hugepages_size})` : ''}
                        </Tag>
                    );
                }
                if (record.dedicated_cpu) {
                    tags.push(
                        <Tag key="dedicated" color="orange">
                            <ThunderboltOutlined /> Dedicated
                        </Tag>
                    );
                }
                return tags.length > 0 ? <Space wrap>{tags}</Space> : <Text type="secondary">—</Text>;
            },
        },
        {
            title: t('instanceSizes.enabled'),
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
                    <Title level={4} style={{ margin: 0 }}>{t('instanceSizes.title')}</Title>
                    <Text type="secondary">{t('instanceSizes.subtitle')}</Text>
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
                    <Table<InstanceSize>
                        columns={columns}
                        dataSource={filteredItems}
                        rowKey="id"
                        loading={isLoading}
                        size="middle"
                        pagination={{
                            total: filteredItems.length,
                            pageSize: 20,
                            showTotal: (total) => t('common:table.total', { total }),
                            showSizeChanger: false,
                        }}
                        locale={{
                            emptyText: (
                                <Empty
                                    description={
                                        deferredSearch
                                            ? t('common:message.no_data')
                                            : t('instanceSizes.empty')
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

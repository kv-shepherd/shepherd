import { describe, expect, it } from 'vitest';

import { buildResolvedInstanceSizePreview } from './resolvedPreview';

describe('buildResolvedInstanceSizePreview', () => {
    it('merges indexed fields with spec overrides into a single preview object', () => {
        const preview = JSON.parse(buildResolvedInstanceSizePreview({
            catalog_scope: 'prod',
            cpu_cores: 8,
            memory_gi: 16,
            disk_gb: 120,
            dedicated_cpu: true,
            cpu_overcommit_enabled: false,
            memory_overcommit_enabled: false,
            requires_sriov: false,
            enabled: true,
            spec_text: JSON.stringify({
                spec: {
                    template: {
                        spec: {
                            domain: {
                                cpu: {
                                    model: 'host-passthrough',
                                },
                                memory: {
                                    hugepages: {
                                        pageSize: '2Mi',
                                    },
                                },
                            },
                        },
                    },
                },
            }),
        })) as {
            spec?: {
                template?: {
                    spec?: {
                        domain?: {
                            cpu?: Record<string, unknown>;
                            memory?: Record<string, unknown>;
                            resources?: Record<string, unknown>;
                        };
                    };
                };
            };
            _platform?: Record<string, unknown>;
        };

        expect(preview.spec?.template?.spec?.domain?.cpu).toMatchObject({
            cores: 8,
            dedicatedCpuPlacement: true,
            model: 'host-passthrough',
        });
        expect(preview.spec?.template?.spec?.domain?.memory).toMatchObject({
            guest: '16Gi',
            hugepages: {
                pageSize: '2Mi',
            },
        });
        expect(preview.spec?.template?.spec?.domain?.resources).toMatchObject({
            limits: {
                cpu: '8',
                memory: '16Gi',
            },
            requests: {
                cpu: '8',
                memory: '16Gi',
            },
        });
        expect(preview._platform).toMatchObject({
            catalog_scope: 'prod',
            disk_gb: 120,
            enabled: true,
            requires_sriov: false,
        });
    });

    it('keeps overcommit request values distinct from limits in preview output', () => {
        const preview = JSON.parse(buildResolvedInstanceSizePreview({
            cpu_cores: 8,
            memory_gi: 16,
            cpu_overcommit_enabled: true,
            cpu_request: 4,
            memory_overcommit_enabled: true,
            memory_request_gi: 8,
            spec_text: '{}',
        })) as {
            spec?: {
                template?: {
                    spec?: {
                        domain?: {
                            resources?: Record<string, unknown>;
                        };
                    };
                };
            };
        };

        expect(preview.spec?.template?.spec?.domain?.resources).toMatchObject({
            limits: {
                cpu: '8',
                memory: '16Gi',
            },
            requests: {
                cpu: '4',
                memory: '8Gi',
            },
        });
    });

    it('drops stale request fields when overcommit is enabled without explicit request values', () => {
        const preview = JSON.parse(buildResolvedInstanceSizePreview({
            cpu_cores: 8,
            memory_gi: 16,
            cpu_overcommit_enabled: true,
            memory_overcommit_enabled: true,
            spec_text: JSON.stringify({
                spec: {
                    template: {
                        spec: {
                            domain: {
                                resources: {
                                    requests: {
                                        cpu: '2',
                                        memory: '4Gi',
                                    },
                                },
                            },
                        },
                    },
                },
            }),
        })) as {
            spec?: {
                template?: {
                    spec?: {
                        domain?: {
                            resources?: {
                                requests?: Record<string, unknown>;
                            };
                        };
                    };
                };
            };
        };

        expect(preview.spec?.template?.spec?.domain?.resources?.requests).toBeUndefined();
    });
});

import { describe, expect, it } from 'vitest';

import {
    buildPodAntiAffinity,
    buildServiceSpreadPodAntiAffinity,
    createDefaultPodAntiAffinityRule,
    parsePodAntiAffinityRule,
} from './podAntiAffinity';

describe('podAntiAffinity helpers', () => {
    it('builds the default required anti-affinity shape used by curated developer sizes', () => {
        expect(buildServiceSpreadPodAntiAffinity()).toEqual({
            requiredDuringSchedulingIgnoredDuringExecution: [
                {
                    labelSelector: {
                        matchExpressions: [
                            {
                                key: 'shepherd.io/service-id',
                                operator: 'In',
                                values: ['__SHEPHERD_SERVICE_ID__'],
                            },
                        ],
                    },
                    topologyKey: 'kubernetes.io/hostname',
                },
            ],
        });
    });

    it('serializes required rules without preferred-only weight or unused values', () => {
        expect(
            buildPodAntiAffinity(
                createDefaultPodAntiAffinityRule({
                    mode: 'required',
                    key: 'app.kubernetes.io/name',
                    operator: 'Exists',
                    values: ['ignored'],
                    topologyKey: 'topology.kubernetes.io/zone',
                }),
            ),
        ).toEqual({
            requiredDuringSchedulingIgnoredDuringExecution: [
                {
                    labelSelector: {
                        matchExpressions: [
                            {
                                key: 'app.kubernetes.io/name',
                                operator: 'Exists',
                            },
                        ],
                    },
                    topologyKey: 'topology.kubernetes.io/zone',
                },
            ],
        });
    });

    it('parses the first supported rule back into the structured form model', () => {
        expect(
            parsePodAntiAffinityRule({
                preferredDuringSchedulingIgnoredDuringExecution: [
                    {
                        weight: 100,
                        podAffinityTerm: {
                            labelSelector: {
                                matchExpressions: [
                                    {
                                        key: 'tenant.example/id',
                                        operator: 'NotIn',
                                        values: ['tenant-a', 'tenant-b'],
                                    },
                                ],
                            },
                            topologyKey: 'topology.kubernetes.io/zone',
                        },
                    },
                ],
            }),
        ).toEqual({
            mode: 'preferred',
            key: 'tenant.example/id',
            operator: 'NotIn',
            values: ['tenant-a', 'tenant-b'],
            topologyKey: 'topology.kubernetes.io/zone',
        });
    });
});

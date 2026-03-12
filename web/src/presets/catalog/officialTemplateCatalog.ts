import type {
    TemplateCreateRequest,
    TemplateUpdateRequest,
} from '@/features/admin-templates/types';

import { curatedLinuxCloudInitExample } from './curatedCatalog';
import type { TemplatePresetCatalogItem } from './types';

export type OfficialTemplatePresetKey =
    | 'official-fedora-eval'
    | 'official-ubuntu-eval'
    | 'official-centos-stream-eval'
    | 'official-opensuse-leap-eval'
    | 'official-debian-eval';

type TemplatePresetFormValues = Partial<TemplateCreateRequest & TemplateUpdateRequest>;

// Official starter templates prefer the portable CDI image import path.
// This keeps the boot source aligned with the KubeVirt/CDI production-friendly
// path while still reusing upstream image sources.
export const OFFICIAL_TEMPLATE_PRESET_ITEMS: Array<TemplatePresetCatalogItem<TemplatePresetFormValues>> = [
    {
        key: 'official-fedora-eval',
        sourceType: 'official',
        verificationLevel: 'verified',
        labelKey: 'templates.official_preset_fedora_eval',
        descriptionKey: 'templates.official_group_description',
        osFamily: 'linux',
        osVersion: 'Fedora',
        imageUrlExamples: ['docker://quay.io/containerdisks/fedora:latest'],
        values: {
            os_family: 'linux',
            os_version: 'Fedora',
            catalog_scope: 'test',
            source_type: 'cdi_image_import',
            image_url: 'docker://quay.io/containerdisks/fedora:latest',
            cloud_init: curatedLinuxCloudInitExample,
            enabled: true,
        },
    },
    {
        key: 'official-ubuntu-eval',
        sourceType: 'official',
        verificationLevel: 'verified',
        labelKey: 'templates.official_preset_ubuntu_eval',
        descriptionKey: 'templates.official_group_description',
        osFamily: 'linux',
        osVersion: 'Ubuntu',
        imageUrlExamples: ['docker://quay.io/containerdisks/ubuntu:latest'],
        values: {
            os_family: 'linux',
            os_version: 'Ubuntu',
            catalog_scope: 'test',
            source_type: 'cdi_image_import',
            image_url: 'docker://quay.io/containerdisks/ubuntu:latest',
            cloud_init: curatedLinuxCloudInitExample,
            enabled: true,
        },
    },
    {
        key: 'official-centos-stream-eval',
        sourceType: 'official',
        verificationLevel: 'verified',
        labelKey: 'templates.official_preset_centos_stream_eval',
        descriptionKey: 'templates.official_group_description',
        osFamily: 'linux',
        osVersion: 'CentOS Stream',
        imageUrlExamples: ['docker://quay.io/containerdisks/centos-stream:latest'],
        values: {
            os_family: 'linux',
            os_version: 'CentOS Stream',
            catalog_scope: 'test',
            source_type: 'cdi_image_import',
            image_url: 'docker://quay.io/containerdisks/centos-stream:latest',
            cloud_init: curatedLinuxCloudInitExample,
            enabled: true,
        },
    },
    {
        key: 'official-opensuse-leap-eval',
        sourceType: 'official',
        verificationLevel: 'verified',
        labelKey: 'templates.official_preset_opensuse_leap_eval',
        descriptionKey: 'templates.official_group_description',
        osFamily: 'linux',
        osVersion: 'openSUSE Leap',
        imageUrlExamples: ['docker://quay.io/containerdisks/opensuse-leap:latest'],
        values: {
            os_family: 'linux',
            os_version: 'openSUSE Leap',
            catalog_scope: 'test',
            source_type: 'cdi_image_import',
            image_url: 'docker://quay.io/containerdisks/opensuse-leap:latest',
            cloud_init: curatedLinuxCloudInitExample,
            enabled: true,
        },
    },
    {
        key: 'official-debian-eval',
        sourceType: 'official',
        verificationLevel: 'verified',
        labelKey: 'templates.official_preset_debian_eval',
        descriptionKey: 'templates.official_group_description',
        osFamily: 'linux',
        osVersion: 'Debian',
        imageUrlExamples: ['docker://quay.io/containerdisks/debian:latest'],
        values: {
            os_family: 'linux',
            os_version: 'Debian',
            catalog_scope: 'test',
            source_type: 'cdi_image_import',
            image_url: 'docker://quay.io/containerdisks/debian:latest',
            cloud_init: curatedLinuxCloudInitExample,
            enabled: true,
        },
    },
];

import { z } from 'zod';

const presetCatalogSourceTypeSchema = z.enum(['curated', 'official', 'imported']);
const presetVerificationLevelSchema = z.enum(['verified', 'experimental', 'unverified']);

export const templatePresetBundleItemSchema = z.object({
    key: z.string().min(1),
    label: z.string().min(1),
    description: z.string().optional(),
    source_type: presetCatalogSourceTypeSchema,
    verification_level: presetVerificationLevelSchema,
    os_family: z.enum(['linux', 'windows']),
    os_version: z.string().min(1),
    values: z.record(z.string(), z.unknown()),
});

export const instanceSizePresetBundleItemSchema = z.object({
    key: z.string().min(1),
    label: z.string().min(1),
    description: z.string().optional(),
    source_type: presetCatalogSourceTypeSchema,
    verification_level: presetVerificationLevelSchema,
    values: z.record(z.string(), z.unknown()),
});

export const presetCatalogBundleSchema = z.object({
    apiVersion: z.literal('shepherd.io/v1alpha1'),
    kind: z.literal('PresetCatalogBundle'),
    metadata: z.object({
        name: z.string().min(1),
        source_type: presetCatalogSourceTypeSchema,
        exported_at: z.string().min(1),
    }),
    items: z.object({
        templates: z.array(templatePresetBundleItemSchema).default([]),
        instance_sizes: z.array(instanceSizePresetBundleItemSchema).default([]),
    }),
});

export type PresetCatalogBundle = z.infer<typeof presetCatalogBundleSchema>;
export type TemplatePresetBundleItem = z.infer<typeof templatePresetBundleItemSchema>;
export type InstanceSizePresetBundleItem = z.infer<typeof instanceSizePresetBundleItemSchema>;

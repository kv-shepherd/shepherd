export type PresetCatalogSourceType = 'curated' | 'official' | 'imported';

export type PresetVerificationLevel = 'verified' | 'experimental' | 'unverified';

export interface TemplatePresetCatalogItem<TValues> {
    key: string;
    sourceType: PresetCatalogSourceType;
    verificationLevel: PresetVerificationLevel;
    labelKey: string;
    descriptionKey?: string;
    osFamily: 'linux' | 'windows';
    osVersion: string;
    pvcNamespace?: string;
    pvcName?: string;
    imageUrlExamples: string[];
    values: TValues;
}

export interface InstanceSizePresetCatalogItem<TValues> {
    key: string;
    sourceType: PresetCatalogSourceType;
    verificationLevel: PresetVerificationLevel;
    labelKey: string;
    descriptionKey?: string;
    values: TValues;
}

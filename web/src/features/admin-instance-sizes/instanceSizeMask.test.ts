import { readFileSync } from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

interface MaskField {
    path: string;
}

interface SchemaMask {
    quick_fields?: MaskField[];
    advanced_fields?: MaskField[];
    professional_fields?: MaskField[];
}

interface ManifestVersionEntry {
    mask_path?: string;
}

interface ManifestEntityEntry {
    current_version?: string;
    versions?: Record<string, ManifestVersionEntry>;
}

interface SchemaManifest {
    entities?: Record<string, ManifestEntityEntry>;
}

const MANIFEST_PATH = path.resolve(
    __dirname,
    '../../../../internal/pkg/schema/manifest.json',
);

function resolveMaskPath(): string {
    const manifest = JSON.parse(
        readFileSync(MANIFEST_PATH, 'utf8'),
    ) as SchemaManifest;
    const instancesize = manifest.entities?.instancesize;
    const currentVersion = instancesize?.current_version;
    const maskPath = currentVersion
        ? instancesize?.versions?.[currentVersion]?.mask_path
        : undefined;
    if (!maskPath) {
        throw new Error('instancesize schema manifest is missing current mask_path');
    }
    return path.resolve(
        __dirname,
        '../../../../internal/pkg/schema',
        maskPath,
    );
}

function resolveAllMaskPaths(): string[] {
    const manifest = JSON.parse(
        readFileSync(MANIFEST_PATH, 'utf8'),
    ) as SchemaManifest;
    const versions = manifest.entities?.instancesize?.versions ?? {};
    return Object.values(versions)
        .map((entry) => entry.mask_path)
        .filter((maskPath): maskPath is string => Boolean(maskPath))
        .map((maskPath) => path.resolve(
            __dirname,
            '../../../../internal/pkg/schema',
            maskPath,
        ));
}

const MASK_PATH = resolveMaskPath();

function readMask(maskPath = MASK_PATH): SchemaMask {
    return JSON.parse(readFileSync(maskPath, 'utf8')) as SchemaMask;
}

function readMaskPaths(maskPath = MASK_PATH): string[] {
    const parsed = readMask(maskPath);
    return [
        ...(parsed.quick_fields ?? []).map((field) => field.path),
        ...(parsed.advanced_fields ?? []).map((field) => field.path),
        ...(parsed.professional_fields ?? []).map((field) => field.path),
    ];
}

describe('instancesize.mask.json', () => {
    it('does not duplicate fields already managed by the parent instance size form', () => {
        const paths = readMaskPaths();

        expect(paths).not.toContain('spec.template.spec.domain.cpu.cores');
        expect(paths).not.toContain('spec.template.spec.domain.memory.guest');
        expect(paths).not.toContain('spec.template.spec.domain.cpu.dedicatedCpuPlacement');
    });

    it('keeps KubeVirt live update ceilings out of guided mask fields', () => {
        const paths = readMaskPaths();

        expect(paths).not.toContain('spec.template.spec.domain.cpu.maxSockets');
        expect(paths).not.toContain('spec.template.spec.domain.memory.maxGuest');
    });

    it('keeps Hugepages Size out of guided mask fields because the Memory section owns it', () => {
        for (const maskPath of resolveAllMaskPaths()) {
            const paths = readMaskPaths(maskPath);

            expect(paths).not.toContain('spec.template.spec.domain.memory.hugepages.pageSize');
        }
    });

    it('promotes high-frequency map fields into the mask', () => {
        const paths = readMaskPaths();

        expect(paths).toContain('spec.template.spec.nodeSelector');
        expect(paths).toContain('spec.template.spec.affinity.podAntiAffinity');
        expect(paths).toContain('spec.template.metadata.annotations');
    });

    it('keeps common tuning fields in advanced and low-frequency knobs in professional', () => {
        const parsed = readMask();
        const advancedPaths = (parsed.advanced_fields ?? []).map((field) => field.path);
        const professionalPaths = (parsed.professional_fields ?? []).map((field) => field.path);

        expect(advancedPaths).toContain('spec.template.spec.domain.cpu.model');
        expect(advancedPaths).toContain('spec.template.spec.domain.cpu.threads');
        expect(advancedPaths).toContain('spec.template.spec.domain.cpu.sockets');
        expect(advancedPaths).toContain('spec.template.spec.domain.devices.autoattachGraphicsDevice');
        expect(advancedPaths).toContain('spec.template.spec.domain.devices.autoattachSerialConsole');
        expect(advancedPaths).toContain('spec.template.spec.domain.devices.autoattachMemBalloon');
        expect(advancedPaths).toContain('spec.template.spec.domain.devices.autoattachVSOCK');
        expect(advancedPaths).toContain('spec.template.spec.domain.devices.rng');

        expect(professionalPaths).toContain('spec.template.spec.domain.features.hyperv.relaxed.enabled');
        expect(professionalPaths).toContain('spec.template.spec.domain.clock.utc');
        expect(professionalPaths).toContain('spec.template.spec.domain.cpu.isolateEmulatorThread');
        expect(professionalPaths).not.toContain('spec.template.spec.domain.devices.autoattachGraphicsDevice');
        expect(professionalPaths).not.toContain('spec.template.spec.domain.devices.autoattachSerialConsole');
        expect(professionalPaths).not.toContain('spec.template.spec.domain.devices.autoattachMemBalloon');
        expect(professionalPaths).not.toContain('spec.template.spec.domain.devices.autoattachVSOCK');
        expect(professionalPaths).not.toContain('spec.template.spec.domain.devices.rng');
    });
});

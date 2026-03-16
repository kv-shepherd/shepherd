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

const MASK_PATH = path.resolve(
    __dirname,
    '../../../../internal/pkg/schema/instancesize.mask.json',
);

function readMask(): SchemaMask {
    return JSON.parse(readFileSync(MASK_PATH, 'utf8')) as SchemaMask;
}

function readMaskPaths(): string[] {
    const parsed = readMask();
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

    it('promotes high-frequency map fields into the mask', () => {
        const paths = readMaskPaths();

        expect(paths).toContain('spec.template.spec.nodeSelector');
        expect(paths).toContain('spec.template.metadata.annotations');
    });

    it('keeps common tuning fields in advanced and low-frequency knobs in professional', () => {
        const parsed = readMask();
        const advancedPaths = (parsed.advanced_fields ?? []).map((field) => field.path);
        const professionalPaths = (parsed.professional_fields ?? []).map((field) => field.path);

        expect(advancedPaths).toContain('spec.template.spec.domain.cpu.model');
        expect(advancedPaths).toContain('spec.template.spec.domain.cpu.threads');
        expect(advancedPaths).toContain('spec.template.spec.domain.cpu.sockets');

        expect(professionalPaths).toContain('spec.template.spec.domain.features.hyperv.relaxed.enabled');
        expect(professionalPaths).toContain('spec.template.spec.domain.clock.utc');
        expect(professionalPaths).toContain('spec.template.spec.domain.cpu.isolateEmulatorThread');
    });
});

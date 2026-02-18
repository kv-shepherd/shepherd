import { describe, expect, it, vi } from 'vitest';

import type { ApiErrorResponse } from './useApiQuery';
import { applyApiFieldErrors } from './applyApiFieldErrors';

describe('applyApiFieldErrors', () => {
  it('maps field_errors to antd form fields', () => {
    const setFields = vi.fn();
    const form = { setFields } as unknown as { setFields: (items: unknown[]) => void };

    const error: ApiErrorResponse = {
      code: 'INVALID_REQUEST',
      field_errors: [
        { field: 'name', code: 'REQUIRED', message: 'name required' },
        { field: 'namespace', code: 'INVALID_FORMAT' },
      ],
    };

    const applied = applyApiFieldErrors(form as never, error);
    expect(applied).toBe(true);
    expect(setFields).toHaveBeenCalledWith([
      { name: ['name'], errors: ['name required'] },
      { name: ['namespace'], errors: ['INVALID_FORMAT'] },
    ]);
  });

  it('returns false when no field errors are present', () => {
    const setFields = vi.fn();
    const form = { setFields } as unknown as { setFields: (items: unknown[]) => void };

    const applied = applyApiFieldErrors(form as never, { code: 'CONFLICT' });
    expect(applied).toBe(false);
    expect(setFields).not.toHaveBeenCalled();
  });
});


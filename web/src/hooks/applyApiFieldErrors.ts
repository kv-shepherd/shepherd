import type { FormInstance } from 'antd';

import type { ApiErrorResponse } from './useApiQuery';

interface FieldErrorSetItem {
    name: string[];
    errors: string[];
}

export function applyApiFieldErrors(
    form: FormInstance,
    error: ApiErrorResponse | null | undefined
): boolean {
    const fieldErrors = error?.field_errors ?? [];
    if (fieldErrors.length === 0) {
        return false;
    }

    const mapped: FieldErrorSetItem[] = fieldErrors
        .filter((item) => item?.field)
        .map((item) => ({
            name: [item.field],
            errors: [item.message || item.code || 'INVALID_FIELD'],
        }));

    if (mapped.length === 0) {
        return false;
    }

    form.setFields(mapped);
    return true;
}


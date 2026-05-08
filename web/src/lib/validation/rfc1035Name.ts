import { z } from 'zod';

const RFC1035_NAME_PATTERN = /^[a-z](?:[a-z0-9]|-[a-z0-9])*$/;

interface Rfc1035NameMessages {
    required: string;
    max: string;
    format: string;
}

interface Rfc1035NameOptions {
    maxLength: number;
}

export function createRfc1035NameSchema(
    messages: Rfc1035NameMessages,
    options: Rfc1035NameOptions,
) {
    return z
        .string({
            error: (issue) => issue.input === undefined ? messages.required : messages.format,
        })
        .min(1, { error: messages.required })
        .max(options.maxLength, { error: messages.max })
        .regex(RFC1035_NAME_PATTERN, { error: messages.format });
}

export function createRfc1035NameRule(
    messages: Rfc1035NameMessages,
    options: Rfc1035NameOptions,
) {
    const schema = createRfc1035NameSchema(messages, options);

    return {
        validator: async (_: unknown, value: unknown) => {
            const result = schema.safeParse(value);
            if (result.success) {
                return;
            }

            throw new Error(result.error.issues[0]?.message ?? messages.format);
        },
    };
}

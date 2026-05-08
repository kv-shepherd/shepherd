import { describe, expect, it } from 'vitest';

import { createRfc1035NameRule, createRfc1035NameSchema } from './rfc1035Name';

const messages = {
    required: 'name is required',
    max: 'name is too long',
    format: 'name has invalid format',
};

describe('createRfc1035NameSchema', () => {
    it('accepts conservative RFC 1035 names', () => {
        const schema = createRfc1035NameSchema(messages, { maxLength: 15 });

        expect(schema.safeParse('a').success).toBe(true);
        expect(schema.safeParse('shop-1').success).toBe(true);
        expect(schema.safeParse('svc9').success).toBe(true);
    });

    it('rejects invalid governance names with localized messages', () => {
        const schema = createRfc1035NameSchema(messages, { maxLength: 15 });

        expect(schema.safeParse(undefined).error?.issues[0]?.message).toBe(messages.required);
        expect(schema.safeParse('').error?.issues[0]?.message).toBe(messages.required);
        expect(schema.safeParse('abcdefghijklmnop').error?.issues[0]?.message).toBe(messages.max);
        expect(schema.safeParse('1shop').error?.issues[0]?.message).toBe(messages.format);
        expect(schema.safeParse('Shop').error?.issues[0]?.message).toBe(messages.format);
        expect(schema.safeParse('shop--api').error?.issues[0]?.message).toBe(messages.format);
        expect(schema.safeParse('shop-').error?.issues[0]?.message).toBe(messages.format);
    });
});

describe('createRfc1035NameRule', () => {
    it('adapts Zod validation to an Ant Design async validator rule', async () => {
        const rule = createRfc1035NameRule(messages, { maxLength: 15 });

        await expect(rule.validator({}, 'shop-1')).resolves.toBeUndefined();
        await expect(rule.validator({}, 'shop--api')).rejects.toThrow(messages.format);
    });
});

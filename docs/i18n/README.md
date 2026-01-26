# Internationalization (i18n)

This directory contains translated documentation for the Shepherd platform.

## Directory Structure

```
i18n/
├── zh-CN/              # Simplified Chinese
│   └── design/
│       └── interaction-flows/
│           └── master-flow.md
├── zh-TW/              # Traditional Chinese (future)
├── ja/                 # Japanese (future)
└── ko/                 # Korean (future)
```

## Language Codes

We use [BCP 47](https://tools.ietf.org/html/bcp47) language tags:

| Code | Language |
|------|----------|
| `zh-CN` | Simplified Chinese (中文简体) |
| `zh-TW` | Traditional Chinese (中文繁體) |
| `ja` | Japanese (日本語) |
| `ko` | Korean (한국어) |

## Contributing Translations

1. Create a directory for your language using the appropriate BCP 47 code
2. Mirror the directory structure of the main docs
3. Translate the content, keeping file names the same
4. Submit a PR with [i18n] prefix in the title

## Canonical Version

The **English version** in `docs/` is always the canonical source of truth.
Translations are provided for convenience and may lag behind the English version.

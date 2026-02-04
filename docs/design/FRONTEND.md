# Frontend Engineering Specification (ADR-0020, ADR-0027)

> **Reference**: [ADR-0020: Frontend Technology Stack](../../adr/ADR-0020-frontend-technology-stack.md)
> **Repository**: `web/` directory (monorepo, ADR-0027)

---

## Technology Stack

| Component | Technology | Version | Notes |
|-----------|------------|---------|-------|
| Framework | Next.js | 15.x | App Router (server components) |
| Language | TypeScript | 5.8+ | Strict mode |
| UI Library | Ant Design | 5.x | Enterprise UI components |
| State Management | Zustand | 5.x | Lightweight state |
| Data Fetching | TanStack Query | 5.x | Server state management |
| i18n | react-i18next | 15.x | Internationalization |
| Form Validation | Zod | 3.x | Schema validation |
| Styling | Tailwind CSS | 4.x | Utility-first CSS |

> **Version Source**: Always refer to [DEPENDENCIES.md](./DEPENDENCIES.md) for pinned versions.

---

## Directory Structure

```
web/
├── src/
│   ├── app/                  # Next.js App Router
│   │   ├── layout.tsx        # Root layout with providers
│   │   ├── page.tsx          # Home page
│   │   ├── (auth)/           # Auth route group
│   │   │   ├── login/
│   │   │   └── logout/
│   │   ├── dashboard/
│   │   ├── systems/
│   │   ├── services/
│   │   ├── vms/
│   │   └── admin/            # Admin routes
│   │       ├── approvals/
│   │       ├── clusters/
│   │       └── users/
│   ├── components/           # Reusable components
│   │   ├── ui/               # Base UI components
│   │   ├── forms/            # Form components
│   │   └── layouts/          # Layout components
│   ├── hooks/                # Custom React hooks
│   ├── lib/                  # Utility functions
│   │   ├── api/              # API client (generated types)
│   │   └── utils/
│   ├── i18n/                 # Internationalization
│   │   ├── index.ts          # i18next initialization
│   │   ├── config.ts         # Language configuration  
│   │   └── locales/          # Translation files
│   │       ├── en/
│   │       └── zh-CN/
│   ├── stores/               # Zustand stores
│   └── types/
│       ├── api.gen.ts        # Generated from OpenAPI (ADR-0021)
│       └── index.ts          # Custom types
├── public/
│   ├── locales/              # Static locale assets (if needed)
│   └── images/
├── package.json
├── tsconfig.json
├── next.config.ts
└── tailwind.config.ts
```

---

## Internationalization (i18n)

### Configuration

```typescript
// src/i18n/config.ts
export const i18nConfig = {
  defaultLocale: 'en',
  locales: ['en', 'zh-CN'],
  fallbackLng: 'en',
  namespaces: ['common', 'errors', 'approval', 'vm', 'admin'],
  defaultNamespace: 'common',
};
```

### Initialization

```typescript
// src/i18n/index.ts
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { i18nConfig } from './config';

// Import locale resources
import enCommon from './locales/en/common.json';
import enErrors from './locales/en/errors.json';
import zhCNCommon from './locales/zh-CN/common.json';
import zhCNErrors from './locales/zh-CN/errors.json';

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      en: {
        common: enCommon,
        errors: enErrors,
      },
      'zh-CN': {
        common: zhCNCommon,
        errors: zhCNErrors,
      },
    },
    ...i18nConfig,
    interpolation: {
      escapeValue: false,
    },
  });

export default i18n;
```

### Error Message Translation

Backend returns error codes with params (per 01-contracts.md):

```json
{
  "code": "NAMESPACE_PERMISSION_DENIED",
  "params": { "namespace": "prod-shop" }
}
```

Frontend translates using these codes:

```json
// src/i18n/locales/en/errors.json
{
  "NAMESPACE_PERMISSION_DENIED": "You don't have permission to create namespace '{{namespace}}'",
  "NAMESPACE_CREATION_FAILED": "Failed to create namespace '{{namespace}}': {{reason}}",
  "CLUSTER_UNHEALTHY": "Cluster '{{cluster}}' is currently unavailable",
  "APPROVAL_REQUIRED": "This action requires approval"
}
```

```json
// src/i18n/locales/zh-CN/errors.json
{
  "NAMESPACE_PERMISSION_DENIED": "您没有创建命名空间 '{{namespace}}' 的权限",
  "NAMESPACE_CREATION_FAILED": "创建命名空间 '{{namespace}}' 失败：{{reason}}",
  "CLUSTER_UNHEALTHY": "集群 '{{cluster}}' 当前不可用",
  "APPROVAL_REQUIRED": "此操作需要审批"
}
```

### Usage in Components

```tsx
import { useTranslation } from 'react-i18next';

function ErrorDisplay({ error }: { error: ApiError }) {
  const { t } = useTranslation('errors');
  
  return (
    <Alert type="error">
      {t(error.code, error.params)}
    </Alert>
  );
}
```

---

## API Type Integration (ADR-0021)

### Generated Types

Types are generated from OpenAPI spec:

```bash
# In project root
make api-generate-ts
# Generates: web/src/types/api.gen.ts
```

### API Client Pattern

```typescript
// src/lib/api/client.ts
import type { paths } from '@/types/api.gen';
import createClient from 'openapi-fetch';

export const api = createClient<paths>({
  baseUrl: process.env.NEXT_PUBLIC_API_URL,
});

// Usage
const { data, error } = await api.GET('/api/v1/vms/{id}', {
  params: { path: { id: vmId } },
});
```

---

## Project Initialization

### Prerequisites

| Requirement | Version |
|-------------|---------|
| Node.js | 22.x LTS |
| pnpm | 9.x |

### Setup Steps

```bash
# Navigate to web directory
cd web

# Install dependencies
pnpm install

# Generate API types (from project root)
cd .. && make api-generate-ts && cd web

# Start development server
pnpm dev
```

### Environment Variables

```bash
# web/.env.local
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_DEFAULT_LOCALE=en
```

---

## Namespace Organization (i18n)

| Namespace | Content | Example Keys |
|-----------|---------|--------------|
| `common` | Shared UI text | `button.submit`, `message.loading` |
| `errors` | Error code translations | `NAMESPACE_PERMISSION_DENIED` |
| `approval` | Approval workflow | `status.pending`, `action.approve` |
| `vm` | VM management | `field.cpu`, `status.running` |
| `admin` | Admin panel | `cluster.add`, `user.list` |

---

## Testing

> **Implementation Guide**: [ADR-0020 Testing Toolchain](./notes/ADR-0020-frontend-testing-toolchain.md)

### Testing Stack

| Layer | Tool | Purpose |
|-------|------|---------|
| **Unit/Component** | Vitest 3.x + React Testing Library 16.x | Fast unit tests, user-centric component testing |
| **Browser Environment** | jsdom | DOM simulation (stable, comprehensive API coverage) |
| **E2E Testing** | Playwright 1.5x | Cross-browser automation, Server Component testing |
| **API Mocking** | MSW 2.x | Service Worker-based API mocking |
| **Coverage** | v8 (via Vitest) | Native V8 coverage, fastest option |

### CI Quality Gates (Mandatory)

| Gate | Tool | Fail Condition |
|------|------|----------------|
| **Type Safety** | TypeScript | Any error with `strict: true` |
| **Lint** | ESLint | Any error |
| **Unit Tests** | Vitest | Any test failure |
| **Coverage** | v8 | **< 80% lines/functions/statements, < 75% branches** |
| **E2E Tests** | Playwright | Any critical path failure |

> ⚠️ **MANDATORY**: Coverage below thresholds will **BLOCK** the PR from merging.

### Test Directories

```
web/
├── src/
│   └── **/*.test.tsx       # Unit tests co-located with source
├── tests/
│   ├── setup.ts            # Vitest setup with MSW
│   ├── mocks/
│   │   └── handlers.ts     # MSW request handlers
│   └── e2e/                # Playwright E2E tests
└── vitest.config.ts
```

### Package.json Scripts

```json
{
  "scripts": {
    "test": "vitest",
    "test:run": "vitest run",
    "test:coverage": "vitest run --coverage",
    "test:e2e": "playwright test"
  }
}
```

---

## Related Documentation

- [ADR-0020: Frontend Technology Stack](../../adr/ADR-0020-frontend-technology-stack.md)
- [ADR-0020: Testing Toolchain Implementation](./notes/ADR-0020-frontend-testing-toolchain.md)
- [ADR-0021: API Contract-First Design](../../adr/ADR-0021-api-contract-first.md)
- [ADR-0027: Monorepo Repository Structure](../../adr/ADR-0027-repository-structure-monorepo.md)
- [01-contracts.md §Error System](./phases/01-contracts.md#6-error-system)
- [DEPENDENCIES.md](./DEPENDENCIES.md)


# Frontend Engineering Specification (ADR-0020, ADR-0027)

> **Reference**: [ADR-0020: Frontend Technology Stack](../../adr/ADR-0020-frontend-technology-stack.md)
> **Repository**: `web/` directory (monorepo, ADR-0027)

---

## Technology Stack

| Component | Technology | Version | Notes |
|-----------|------------|---------|-------|
| Core Library | React | 19.x | Required by Next.js 15 |
| Framework | Next.js | 15.x | App Router (server components) |
| Language | TypeScript | 5.8+ | Strict mode |
| UI Library | Ant Design | 5.x | Enterprise UI components |
| State Management | Zustand | 5.x | Lightweight state |
| Data Fetching | TanStack Query | 5.x | Server state management |
| i18n | react-i18next | 15.x | Internationalization |
| Form Validation | Zod | 3.x | Schema validation |
| Styling | Tailwind CSS | 4.x | Utility-first CSS |

> **Version Source**: Always refer to [DEPENDENCIES.md](../DEPENDENCIES.md) for pinned versions.

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

## Async Batch Queue UX (ADR-0015 §19)

Batch operations follow the parent-child ticket model.

Mandatory frontend behavior:

- Render parent aggregate status and child execution details.
- Track status through backend `status_url` endpoints until terminal parent state.
- Support `retry failed children` and `terminate pending children`.
- Handle `429 Too Many Requests` with `Retry-After` based cooldown.

Detailed specification:

- [Batch Operations Queue UI](./features/batch-operations-queue.md)
- [master-flow.md Stage 5.E](../interaction-flows/master-flow.md#stage-5e-batch-operations)

---

## Schema Cache Degradation Strategy (ADR-0023)

> **Reference**: [ADR-0023: Schema Cache Management](../../adr/ADR-0023-schema-cache-and-api-standards.md)

The Schema-Driven UI pattern relies on KubeVirt JSON Schema for dynamic form rendering. This section defines **mandatory** degradation behaviors when schema cache fails or version drifts.

### Core Principles

| Principle | Description |
|-----------|-------------|
| **Progressive Enhancement** | Core functionality (basic CPU/Memory) always works; advanced features (GPU, SR-IOV) require schema |
| **Stale-While-Revalidate** | Serve cached schema immediately; refresh in background |
| **Explicit User Feedback** | Never fail silently; always inform users of degraded state |
| **Multi-Layer Fallback** | Memory → IndexedDB → Embedded Default → Remote Fetch |

### API Response Headers

Backend MUST return schema status via HTTP headers:

| Header | Value | Frontend Action |
|--------|-------|-----------------|
| `X-Schema-Version` | `1.5.x` | Display version in form footer |
| `X-Schema-Fallback` | `embedded-v1.4.x` | Show warning banner |
| `X-Schema-Status` | `updating` | Show loading indicator |

### Frontend Caching Implementation

```typescript
// src/lib/schema/cache.ts
interface SchemaCache {
  get(version: string): Promise<JSONSchema | null>;
  set(version: string, schema: JSONSchema): Promise<void>;
  fallback(): JSONSchema;  // Embedded default schema
}

// Priority: Memory → IndexedDB → Embedded → Remote
async function getSchema(version: string): Promise<{
  schema: JSONSchema;
  source: 'cache' | 'embedded' | 'remote';
}> {
  // 1. Try memory cache
  const cached = memoryCache.get(version);
  if (cached) return { schema: cached, source: 'cache' };
  
  // 2. Try IndexedDB
  const stored = await idbCache.get(version);
  if (stored) {
    memoryCache.set(version, stored);
    return { schema: stored, source: 'cache' };
  }
  
  // 3. Try embedded fallback (bundled at build time)
  const embedded = EMBEDDED_SCHEMAS[minorVersion(version)];
  if (embedded) return { schema: embedded, source: 'embedded' };
  
  // 4. Fetch from server (last resort)
  const fetched = await api.GET('/api/v1/schema/{version}', { params: { path: { version } } });
  if (fetched.data) {
    await idbCache.set(version, fetched.data);
    return { schema: fetched.data, source: 'remote' };
  }
  
  throw new SchemaUnavailableError(version);
}
```

### UI Degradation States

| State | Trigger | UI Behavior |
|-------|---------|-------------|
| **Normal** | Schema from cache | Full dynamic form |
| **Fallback** | Using embedded/older schema | ⚠️ Warning banner + full form |
| **Updating** | Background fetch in progress | 🔄 Loading indicator in header |
| **Degraded** | No schema available | Basic fields only + error alert |

### Degraded Mode UI (Mandatory)

When schema is unavailable, render **basic fields only**:

```tsx
// src/components/forms/InstanceSizeForm.tsx
function InstanceSizeForm({ schemaState }: Props) {
  if (schemaState.status === 'unavailable') {
    return (
      <Alert type="warning" showIcon>
        <AlertTitle>{t('schema.unavailable.title')}</AlertTitle>
        <AlertDescription>
          {t('schema.unavailable.description')}
        </AlertDescription>
      </Alert>
      <BasicFieldsForm />  {/* CPU, Memory only */}
      <Text type="secondary">
        {t('schema.unavailable.advanced_hidden')}
      </Text>
    );
  }
  // ... normal dynamic form
}
```

### i18n Keys (Required)

```json
// src/i18n/locales/en/common.json
{
  "schema.unavailable.title": "Schema Unavailable",
  "schema.unavailable.description": "Unable to load KubeVirt schema for version {{version}}. Advanced fields are hidden.",
  "schema.unavailable.advanced_hidden": "GPU, Hugepages, SR-IOV options are temporarily unavailable.",
  "schema.fallback_warning": "Using fallback schema ({{version}}). Some features may be limited.",
  "schema.updating": "Updating schema..."
}
```

### Admin Notifications

Schema failures MUST trigger admin notifications:

| Condition | Notification Level | Delivery |
|-----------|-------------------|----------|
| Cache miss (using embedded) | Warning | Dashboard widget |
| Fetch failed 3+ times | Error | In-app notification |
| Version drift detected | Info | Audit log only |

> **Implementation Note**: Use `useSchemaStatus()` hook for consistent schema state management across components.

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

> **Implementation Guide**: [ADR-0020 Testing Toolchain](../notes/ADR-0020-frontend-testing-toolchain.md)

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

- [Frontend Design Index](./README.md)
- [Batch Queue UI (Parent-Child)](./features/batch-operations-queue.md)
- [ADR-0030: Design Documentation Layering](../../adr/ADR-0030-design-documentation-layering-and-fullstack-governance.md)
- [ADR-0020: Frontend Technology Stack](../../adr/ADR-0020-frontend-technology-stack.md)
- [ADR-0020: Testing Toolchain Implementation](../notes/ADR-0020-frontend-testing-toolchain.md)
- [ADR-0021: API Contract-First Design](../../adr/ADR-0021-api-contract-first.md)
- [ADR-0027: Monorepo Repository Structure](../../adr/ADR-0027-repository-structure-monorepo.md)
- [01-contracts.md §Error System](../phases/01-contracts.md#6-error-system)
- [DEPENDENCIES.md](../DEPENDENCIES.md)

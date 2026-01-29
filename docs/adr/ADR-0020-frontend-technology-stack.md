---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-01-27
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-0020: Frontend Technology Stack Selection

> **Review Period**: Until 2026-01-30 (48-hour minimum)  
> **Discussion**: [Issue #30](https://github.com/kv-shepherd/shepherd/issues/30)  
> **Related**: [ADR-0018](./ADR-0018-instance-size-abstraction.md) (Schema-Driven UI requirement)

---

## Context and Problem Statement

KubeVirt Shepherd requires a frontend application to provide a user interface for:

1. **Regular Users**: System/Service/VM management, request submission
2. **Approvers**: Approval workflow, cluster selection, parameter overrides
3. **Platform Admins**: Cluster configuration, Template/InstanceSize management, RBAC configuration

The frontend must support the **Schema-Driven UI** pattern defined in [ADR-0018](./ADR-0018-instance-size-abstraction.md), where:
- KubeVirt JSON Schema defines field types and constraints
- Mask configuration selects which paths to expose
- Frontend dynamically renders appropriate form components

We need to select a technology stack that:
- Supports enterprise-grade admin dashboard requirements
- Enables Schema-Driven dynamic form rendering
- Provides long-term maintainability and community support
- Aligns with Kubernetes ecosystem conventions

---

## Decision Drivers

* **Enterprise-grade requirements**: Complex tables, forms, RBAC, audit logs, i18n
* **Schema-Driven UI support**: Dynamic form generation from JSON Schema
* **Ecosystem maturity**: Large community, extensive libraries, proven stability
* **Kubernetes ecosystem alignment**: Consistency with other K8s management tools (Lens, ArgoCD, Headlamp)
* **Developer availability**: Sufficient talent pool for long-term maintenance
* **TypeScript support**: Type safety to complement Go backend's strict typing
* **Performance**: Handle data-intensive dashboards with real-time updates

---

## Considered Options

* **Option 1**: React + Next.js + Ant Design
* **Option 2**: Vue + Nuxt + Element Plus
* **Option 3**: SolidJS + Custom Components
* **Option 4**: Angular + Angular Material

---

## Decision Outcome

**Recommended option**: "Option 1: React + Next.js + Ant Design", because it provides the best combination of ecosystem maturity, enterprise component quality, Schema-Driven form support, and Kubernetes ecosystem alignment.

### Complete Technology Stack

| Layer | Technology | Version | Rationale |
|-------|------------|---------|-----------|
| **Language** | TypeScript | 5.x | Type safety, IDE support, complements Go backend |
| **Framework** | React | 19.x | Largest ecosystem, enterprise-proven, K8s ecosystem standard |
| **Meta-Framework** | Next.js | 15.x (App Router) | SSR/SSG, built-in optimizations, API routes for BFF |
| **UI Components** | Ant Design | 5.x | 100+ enterprise components, ProComponents extension |
| **Extended Components** | @ant-design/pro-components | 2.x | ProTable, ProForm for complex data handling |
| **State Management** | Zustand | 5.x | Lightweight, TypeScript-native, simple API |
| **Server State** | TanStack Query | 5.x | Best-in-class server state management, caching |
| **Schema Validation** | Zod | 3.x | TypeScript-first schema validation |
| **Charts** | ECharts (echarts-for-react) | 5.x | Comprehensive charting, good CJK support |
| **Internationalization** | react-i18next | 15.x | Mature i18n solution |
| **Styling** | CSS Modules or Tailwind CSS | 4.x | Team preference |
| **Testing** | Vitest + Testing Library | - | Fast, modern testing |
| **E2E Testing** | Playwright | - | Cross-browser E2E |

### Consequences

* ✅ Good, because React has the largest ecosystem and community support
* ✅ Good, because Ant Design provides comprehensive enterprise components out of the box
* ✅ Good, because @ant-design/pro-form supports JSON Schema-driven form rendering
* ✅ Good, because Next.js provides SSR/SSG for SEO and performance optimization
* ✅ Good, because most Kubernetes dashboard tools use React (Lens, ArgoCD, Headlamp)
* ✅ Good, because TypeScript ensures type safety across the codebase
* 🟡 Neutral, because React has a steeper learning curve than Vue
* 🟡 Neutral, because Ant Design's visual style may require customization for branding
* ❌ Bad, because bundle size may be larger than minimal alternatives (mitigated by Next.js optimizations)

### Confirmation

* Technology stack validated through proof-of-concept for Schema-Driven form rendering
* Performance benchmarks meet requirements for data-intensive tables (1000+ rows)
* Accessibility audit passes WCAG 2.1 AA standards
* Build and deployment pipeline successfully configured

### Alternatives Also Considered for UI Components

While Ant Design is the recommended choice, the following alternatives were evaluated:

| Library | Pros | Cons | Conclusion |
|---------|------|------|------------|
| **Shadcn UI + Radix** | Maximum customizability, smaller bundle, modern trends | More initial setup, less enterprise components | Consider for future if branding diverges significantly |
| **Mantine** | Modern, TypeScript-first, comprehensive hooks | Smaller community, less enterprise precedent | Good alternative but less proven at scale |
| **Blueprint UI** | Dense desktop interfaces, professional applications | Heavier, less Schema-driven support | Too specialized for general use |

**Why Ant Design was chosen**: `@ant-design/pro-components` provides Schema-driven form rendering (`ProForm`) out of the box, which directly supports ADR-0018 requirements. Building equivalent functionality from primitives would delay the project significantly.

---

## Repository Structure

### Separate Repository

The frontend will be maintained in a **separate repository** (`shepherd-ui` or `shepherd-web`) for:

1. **Independent versioning**: Frontend and backend can release independently
2. **Clear ownership**: Separate CI/CD pipelines and maintainers
3. **Technology isolation**: Frontend tooling doesn't pollute backend workspace
4. **Standard practice**: Consistent with microservices architecture

### Proposed Directory Structure

```
shepherd-ui/
├── src/
│   ├── app/                          # Next.js App Router
│   │   ├── (auth)/                   # Auth pages (login, callback)
│   │   ├── (dashboard)/              # Main dashboard
│   │   │   ├── systems/
│   │   │   ├── services/
│   │   │   ├── vms/
│   │   │   ├── approvals/
│   │   │   └── admin/
│   │   │       ├── clusters/
│   │   │       ├── templates/
│   │   │       ├── instance-sizes/
│   │   │       └── rbac/
│   │   ├── api/                      # BFF API routes
│   │   └── layout.tsx
│   │
│   ├── components/                   # Reusable components
│   │   ├── ui/                       # Base UI components
│   │   ├── forms/                    # Schema-Driven forms
│   │   ├── tables/                   # Data tables
│   │   └── layouts/                  # Layout components
│   │
│   ├── features/                     # Feature modules
│   │   ├── vm/
│   │   ├── approval/
│   │   ├── system/
│   │   └── auth/
│   │
│   ├── lib/                          # Utilities
│   │   ├── api/                      # API client (fetch wrapper)
│   │   ├── hooks/                    # Custom hooks
│   │   ├── schema/                   # JSON Schema utilities
│   │   └── utils/
│   │
│   ├── stores/                       # Zustand stores
│   └── types/                        # TypeScript types
│
├── public/                           # Static assets
├── tests/
│   ├── unit/                         # Unit tests
│   └── e2e/                          # Playwright E2E tests
│
├── .github/                          # GitHub Actions
├── next.config.js
├── package.json
├── tsconfig.json
└── README.md
```

---

## Schema-Driven UI Implementation

Per [ADR-0018](./ADR-0018-instance-size-abstraction.md), the frontend must dynamically render forms based on:

1. **KubeVirt JSON Schema**: Defines field types, constraints, enums
2. **Mask Configuration**: Specifies which Schema paths to expose

### Implementation Approach

```typescript
// lib/schema/schema-form.tsx
import { ProForm, ProFormText, ProFormSelect, ProFormDigit } from '@ant-design/pro-components';
import type { JSONSchema7 } from 'json-schema';

interface MaskConfig {
  exposedPaths: string[];
  quickFields: string[];
  advancedFields: string[];
}

interface SchemaFormProps {
  schema: JSONSchema7;
  mask: MaskConfig;
  initialValues?: Record<string, unknown>;
  onFinish: (values: Record<string, unknown>) => Promise<void>;
  mode?: 'quick' | 'advanced';
}

export function SchemaForm({ schema, mask, initialValues, onFinish, mode = 'quick' }: SchemaFormProps) {
  const visiblePaths = mode === 'quick' ? mask.quickFields : [...mask.quickFields, ...mask.advancedFields];
  const fields = extractFieldsFromSchema(schema, visiblePaths);

  return (
    <ProForm initialValues={initialValues} onFinish={onFinish}>
      {fields.map(field => (
        <SchemaField key={field.path} field={field} />
      ))}
    </ProForm>
  );
}

function SchemaField({ field }: { field: ExtractedField }) {
  switch (field.type) {
    case 'string':
      return field.enum 
        ? <ProFormSelect name={field.path} label={field.title} options={field.enum.map(v => ({ label: v, value: v }))} />
        : <ProFormText name={field.path} label={field.title} />;
    case 'integer':
    case 'number':
      return <ProFormDigit name={field.path} label={field.title} min={field.minimum} max={field.maximum} />;
    case 'boolean':
      return <ProFormSwitch name={field.path} label={field.title} />;
    default:
      return null;
  }
}
```

### Key Libraries for Schema-Driven Forms

| Library | Purpose |
|---------|---------|
| `@ant-design/pro-form` | Advanced form components with validation |
| `ajv` | JSON Schema validation |
| `json-schema-to-typescript` | Generate TypeScript types from Schema |

---

## React/Next.js Design Patterns

Based on modern React best practices, the following patterns should be applied throughout the codebase:

### 6 Core Design Patterns

| Pattern | Description | When to Use |
|---------|-------------|-------------|
| **Specialized Component Extraction** | Break large components into smaller, focused pieces | Complex components with multiple responsibilities |
| **Compound Components** | Parent with sub-components that work together | Flexible, composable UI (Form.Header, Form.Content) |
| **Config Objects** | Group related props into logical objects | Components with many related props |
| **Component Composition** | Build complex UIs from smaller, reusable pieces | Avoiding conditional prop explosion |
| **Separation of Concerns** | Each component has single responsibility | All components |
| **Slots Pattern** | Named slots for flexible content placement | Customizable layouts |

### Pattern Application Examples

#### 1. Compound Components for Forms

```typescript
// components/forms/approval-form.tsx
import { createContext, useContext } from 'react';

interface ApprovalFormContextType {
  formState: FormState;
  setField: (name: string, value: unknown) => void;
}

const ApprovalFormContext = createContext<ApprovalFormContextType | undefined>(undefined);

function useApprovalForm() {
  const context = useContext(ApprovalFormContext);
  if (!context) throw new Error('Must be used within ApprovalForm');
  return context;
}

// Parent component
export function ApprovalForm({ children, onSubmit }: ApprovalFormProps) {
  const [formState, setFormState] = useState<FormState>({});
  
  const setField = (name: string, value: unknown) => {
    setFormState(prev => ({ ...prev, [name]: value }));
  };

  return (
    <ApprovalFormContext.Provider value={{ formState, setField }}>
      <form onSubmit={() => onSubmit(formState)}>
        {children}
      </form>
    </ApprovalFormContext.Provider>
  );
}

// Sub-components
ApprovalForm.ClusterSelect = function ClusterSelect() {
  const { formState, setField } = useApprovalForm();
  return <Select value={formState.clusterId} onChange={v => setField('clusterId', v)} />;
};

ApprovalForm.InstanceSizeSelect = function InstanceSizeSelect() {
  const { formState, setField } = useApprovalForm();
  return <Select value={formState.instanceSize} onChange={v => setField('instanceSize', v)} />;
};

ApprovalForm.Actions = function Actions() {
  return (
    <div className="flex gap-2">
      <Button type="submit" variant="primary">Approve</Button>
      <Button type="button" variant="secondary">Reject</Button>
    </div>
  );
};

// Usage - flexible composition
<ApprovalForm onSubmit={handleApprove}>
  <ApprovalForm.ClusterSelect />
  <ApprovalForm.InstanceSizeSelect />
  <ApprovalForm.Actions />
</ApprovalForm>
```

#### 2. Custom Hooks for Reusability

```typescript
// hooks/use-vm-list.ts
export function useVMList(serviceId: string, options?: QueryOptions) {
  return useQuery({
    queryKey: ['vms', serviceId],
    queryFn: () => api.vms.list(serviceId),
    staleTime: 30_000, // 30 seconds
    ...options,
  });
}

// hooks/use-approval.ts
export function useApproval(requestId: string) {
  const queryClient = useQueryClient();
  
  const approve = useMutation({
    mutationFn: (data: ApprovalData) => api.approvals.approve(requestId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['requests'] });
      toast.success('Request approved');
    },
  });
  
  const reject = useMutation({
    mutationFn: (reason: string) => api.approvals.reject(requestId, reason),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['requests'] });
    },
  });

  return { approve, reject };
}

// Usage in component - no prop drilling needed
function ApprovalActions({ requestId }: { requestId: string }) {
  const { approve, reject } = useApproval(requestId);
  // ...
}
```

#### 3. Config Objects to Reduce Prop Explosion

```typescript
// ❌ Bad - too many props
<VMCard
  showStatus
  showActions
  showMetrics
  allowEdit
  allowDelete
  isSelectable
  isHighlighted
  variant="compact"
  theme="dark"
/>

// ✅ Good - grouped config
interface VMCardConfig {
  display: {
    showStatus?: boolean;
    showActions?: boolean;
    showMetrics?: boolean;
  };
  permissions: {
    allowEdit?: boolean;
    allowDelete?: boolean;
  };
  appearance: {
    variant?: 'compact' | 'full';
    isSelectable?: boolean;
    isHighlighted?: boolean;
  };
}

<VMCard config={{
  display: { showStatus: true, showActions: true },
  permissions: { allowEdit: true },
  appearance: { variant: 'compact' }
}} />
```

#### 4. Server Actions for Mutations (Next.js 15)

```typescript
// app/actions/vm.ts
'use server'

import { revalidatePath } from 'next/cache';
import { z } from 'zod';

const CreateVMSchema = z.object({
  serviceId: z.string().uuid(),
  templateId: z.string().uuid(),
  name: z.string().min(3).max(63),
});

export async function createVM(formData: FormData) {
  const validated = CreateVMSchema.safeParse({
    serviceId: formData.get('serviceId'),
    templateId: formData.get('templateId'),
    name: formData.get('name'),
  });

  if (!validated.success) {
    return { error: validated.error.flatten() };
  }

  const result = await api.vms.create(validated.data);
  revalidatePath('/vms');
  return { success: true, vm: result };
}

// Usage in client component
function CreateVMForm() {
  const [state, formAction] = useActionState(createVM, null);
  
  return (
    <form action={formAction}>
      {/* form fields */}
      <SubmitButton />
    </form>
  );
}
```

### Best Practice Recommendations

| Priority | Recommendation |
|----------|----------------|
| 1 | **Always use TypeScript** - Strict types prevent most issues |
| 2 | **Use Composition** - Break components into smaller pieces |
| 3 | **Extract Custom Hooks** - Logic in hooks, UI in components |
| 4 | **Group Related Props** - Use config objects for complex components |
| 5 | **Use Context Sparingly** - Only for deeply nested data |
| 6 | **Validate at Boundaries** - Use Zod for runtime validation |

---

## Code Quality and Debuggability

To ensure long-term maintainability and ease of debugging, the frontend must adhere to strict code quality standards enforced through CI/CD pipelines.

### CI Quality Gates (Mandatory)

The following checks MUST pass before any PR can be merged:

| Check | Tool | Fail Condition |
|-------|------|----------------|
| **Type Safety** | TypeScript | Any error with `strict: true` |
| **Lint** | ESLint | Any error (warnings allowed in dev) |
| **Format** | Prettier | Any unformatted file |
| **Unit Tests** | Vitest | Coverage < 70% for new code |
| **Build** | Next.js | Build failure |

### TypeScript Configuration (tsconfig.json)

```json
{
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
    "exactOptionalPropertyTypes": true,
    "verbatimModuleSyntax": true
  }
}
```

**Rationale**: Strict TypeScript catches errors at compile time rather than runtime, significantly reducing debugging time.

### ESLint Configuration

Use ESLint Flat Config (eslint.config.js) with these mandatory rules:

```javascript
// eslint.config.js
import eslint from '@eslint/js';
import tseslint from 'typescript-eslint';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';

export default tseslint.config(
  eslint.configs.recommended,
  ...tseslint.configs.strictTypeChecked,
  {
    rules: {
      // Complexity limits for debuggability
      'max-lines-per-function': ['error', { max: 100, skipBlankLines: true, skipComments: true }],
      'max-depth': ['error', 3],
      'complexity': ['error', 10],
      
      // Explicit is better than implicit
      '@typescript-eslint/explicit-function-return-type': 'error',
      '@typescript-eslint/explicit-module-boundary-types': 'error',
      '@typescript-eslint/no-explicit-any': 'error',
      
      // React best practices
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      
      // Import organization
      'import/order': ['error', {
        'groups': ['builtin', 'external', 'internal', 'parent', 'sibling', 'index'],
        'newlines-between': 'always'
      }]
    }
  }
);
```

### Component Design Principles

#### 1. Single Responsibility

Each component should have ONE clear purpose. Split large components.

```typescript
// ❌ Bad: Component doing too much
function VMCard({ vm }: { vm: VM }) {
  // Fetching, formatting, rendering, actions all in one
}

// ✅ Good: Clear separation
function VMCard({ vm }: { vm: VM }) {
  return (
    <Card>
      <VMCardHeader name={vm.name} status={vm.status} />
      <VMCardMetrics vm={vm} />
      <VMCardActions vmId={vm.id} />
    </Card>
  );
}
```

#### 2. Explicit Over Implicit

Prefer verbose but clear code over clever but obscure patterns.

```typescript
// ❌ Avoid: Chained operations hard to debug
const result = data.filter(x => x.active).map(x => x.name).slice(0, 10);

// ✅ Prefer: Step-by-step for easy breakpoint debugging
const activeItems = data.filter(item => item.active);
const itemNames = activeItems.map(item => item.name);
const result = itemNames.slice(0, 10);
```

#### 3. Direct Imports

Always use direct imports instead of barrel files for better stack traces.

```typescript
// ❌ Avoid: Barrel import
import { Button, Input, Select } from '@/components/ui';

// ✅ Prefer: Direct imports
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
```

#### 4. Typed Custom Hooks

Extract complex logic into well-typed custom hooks.

```typescript
// ✅ Good: Logic encapsulated with clear types
interface UseVMListResult {
  vms: VM[];
  isLoading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

function useVMList(serviceId: string): UseVMListResult {
  // Implementation
}
```

### Directory Structure Enforcement

```
src/
├── app/                    # Next.js App Router (pages only)
│   └── (feature)/          # Route groups with feature name
├── components/             # Reusable UI components
│   ├── ui/                 # Base components (Button, Input, etc.)
│   └── feature/            # Feature-specific components
├── features/               # Business logic organized by domain
│   └── {feature}/
│       ├── hooks/          # Feature-specific hooks
│       ├── types/          # Feature-specific types
│       └── utils/          # Feature-specific utilities
├── lib/                    # Shared utilities
│   ├── api/                # API client and types
│   ├── hooks/              # Shared hooks
│   └── utils/              # Shared utilities
├── stores/                 # Zustand stores
└── types/                  # Global TypeScript types
```

**Rules**:
- Components in `app/` should be thin wrappers around feature components
- No business logic in `components/ui/`
- Each feature folder should be self-contained
- Cross-feature imports must go through `lib/`

### Error Handling Standards

All async operations must have explicit error handling:

```typescript
// ✅ Good: Complete error context
try {
  const result = await api.vms.create(data);
  return { success: true, data: result };
} catch (error) {
  console.error('[VMCreate] Failed to create VM:', {
    input: data,
    error: error instanceof Error ? error.message : String(error),
    stack: error instanceof Error ? error.stack : undefined,
  });
  return { success: false, error: 'Failed to create VM' };
}
```

### Pre-commit Hooks

Use Husky + lint-staged for automated checks before commit:

```json
// package.json
{
  "lint-staged": {
    "*.{ts,tsx}": ["eslint --fix", "prettier --write"],
    "*.{json,md}": ["prettier --write"]
  }
}
```

---

## Performance Optimization Guidelines

This section defines mandatory performance patterns based on Vercel Engineering best practices. These patterns are prioritized by impact.

### Critical: Eliminating Waterfalls

Waterfalls (sequential async operations) are the #1 performance killer.

```typescript
// ❌ Bad: Sequential execution (3 round trips)
const user = await fetchUser()
const posts = await fetchPosts()
const comments = await fetchComments()

// ✅ Good: Parallel execution (1 round trip)
const [user, posts, comments] = await Promise.all([
  fetchUser(),
  fetchPosts(),
  fetchComments()
])
```

**Rule**: Always use `Promise.all()` for independent async operations.

### Critical: Bundle Size Optimization

Reducing initial bundle size improves Time to Interactive (TTI).

**Avoid Barrel File Imports**:

```typescript
// ❌ Bad: Imports entire library
import { Button, Table } from 'antd'

// ✅ Good: Direct imports (when not using optimizePackageImports)
import Button from 'antd/es/button'
import Table from 'antd/es/table'
```

**Next.js Configuration** (enables ergonomic imports):

```javascript
// next.config.js
module.exports = {
  experimental: {
    optimizePackageImports: ['antd', '@ant-design/icons', 'lodash-es']
  }
}
```

**Dynamic Imports for Heavy Components**:

```typescript
import dynamic from 'next/dynamic'

// Load Monaco editor only when needed (~300KB)
const MonacoEditor = dynamic(
  () => import('./monaco-editor').then(m => m.MonacoEditor),
  { ssr: false, loading: () => <Skeleton /> }
)
```

### High: Server Component Optimization

**Minimize Data at RSC Boundaries**:

```typescript
// ❌ Bad: Serializes all 50 fields
async function Page() {
  const user = await fetchUser()  // 50 fields
  return <Profile user={user} />  // sends all to client
}

// ✅ Good: Select only needed fields
async function Page() {
  const user = await fetchUser()
  return <Profile name={user.name} avatar={user.avatar} />
}
```

**Use Suspense for Streaming**:

```tsx
function Page() {
  return (
    <div>
      <Header />  {/* Renders immediately */}
      <Suspense fallback={<TableSkeleton />}>
        <VMTable />  {/* Streams in when ready */}
      </Suspense>
      <Footer />  {/* Renders immediately */}
    </div>
  )
}
```

### Medium: Re-render Optimization

**Calculate Derived State During Rendering**:

```typescript
// ❌ Bad: useEffect for derived state
const [items, setItems] = useState([])
const [filteredItems, setFilteredItems] = useState([])

useEffect(() => {
  setFilteredItems(items.filter(i => i.active))
}, [items])

// ✅ Good: Derive during render
const [items, setItems] = useState([])
const filteredItems = useMemo(
  () => items.filter(i => i.active),
  [items]
)
```

**Use Functional setState**:

```typescript
// ❌ Bad: Closure captures stale value
const handleClick = () => setCount(count + 1)

// ✅ Good: Always gets latest value
const handleClick = () => setCount(prev => prev + 1)
```

### Documentation Reference

For the complete performance optimization ruleset (57 rules across 8 categories), see:
- `.agent/skills/vercel-react-best-practices/AGENTS.md`

---

## Pros and Cons of the Options

### Option 1: React + Next.js + Ant Design (Recommended)

* ✅ Good, because Ant Design has 100+ enterprise-ready components
* ✅ Good, because ProComponents provides built-in Schema-driven forms (ProForm)
* ✅ Good, because React is the most used framework in K8s ecosystem (Lens, ArgoCD, Headlamp)
* ✅ Good, because Next.js offers SSR, SSG, and built-in optimizations
* ✅ Good, because largest developer talent pool globally
* 🟡 Neutral, because steeper learning curve than Vue
* ❌ Bad, because Ant Design's distinct style may need customization

### Option 2: Vue + Nuxt + Element Plus

* ✅ Good, because Vue has gentler learning curve
* ✅ Good, because Element Plus provides solid enterprise components
* ✅ Good, because strong adoption in Asian markets
* 🟡 Neutral, because ecosystem smaller than React's
* ❌ Bad, because fewer K8s dashboard precedents use Vue
* ❌ Bad, because Schema-driven form solutions less mature than React's

### Option 3: SolidJS + Custom Components

* ✅ Good, because highest raw performance (50-70% faster than React)
* ✅ Good, because smallest bundle size (~5KB)
* ❌ Bad, because ecosystem still maturing
* ❌ Bad, because no enterprise-grade component library available
* ❌ Bad, because smaller talent pool increases hiring risk

### Option 4: Angular + Angular Material

* ✅ Good, because strong TypeScript integration
* ✅ Good, because official Kubernetes Dashboard uses Angular
* 🟡 Neutral, because steepest learning curve of all options
* ❌ Bad, because declining market share compared to React
* ❌ Bad, because Angular Material less comprehensive than Ant Design

---

## Acceptance Checklist (Execution Tasks)

Upon acceptance, perform the following:

1. [ ] Create new repository `shepherd-ui` (or `kv-shepherd/shepherd-ui`)
2. [ ] Initialize project with `npx create-next-app@latest --typescript --app --tailwind=false`
3. [ ] Configure base dependencies:
   - `npm install antd @ant-design/pro-components`
   - `npm install zustand @tanstack/react-query`
   - `npm install zod react-i18next`
4. [ ] Set up CI/CD pipeline (GitHub Actions)
5. [ ] Create proof-of-concept for Schema-Driven form
6. [ ] Document API contract with backend

---

## References

* [Ant Design Official Documentation](https://ant.design/)
* [Next.js App Router Documentation](https://nextjs.org/docs/app)
* [TanStack Query Documentation](https://tanstack.com/query)
* [Next.js Best Practices Guide](https://github.com/bablukpik/nextjs-best-practices) - Design patterns reference
* [Kubernetes Dashboard (Angular reference)](https://github.com/kubernetes/dashboard)
* [Lens Desktop (React reference)](https://github.com/lensapp/lens)
* [Headlamp (React reference)](https://github.com/headlamp-k8s/headlamp)
* [ArgoCD UI (React reference)](https://github.com/argoproj/argo-cd)
* [ADR-0018: Instance Size Abstraction](./ADR-0018-instance-size-abstraction.md)
* [ADR-0021: API Contract-First Design](./ADR-0021-api-contract-first.md)

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-28 | @jindyzhao | Added Code Quality and Debuggability section with CI quality gates |
| 2026-01-28 | @jindyzhao | Added Performance Optimization Guidelines based on Vercel Engineering best practices |
| 2026-01-28 | @jindyzhao | Added Alternatives Also Considered section for UI component libraries |

| 2026-01-27 | @jindyzhao | Added React/Next.js Design Patterns section based on nextjs-best-practices |
| 2026-01-27 | @jindyzhao | Initial draft based on 2026 best practices research |

---

_End of ADR-0020_

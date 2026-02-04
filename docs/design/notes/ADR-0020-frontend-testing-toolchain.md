# ADR-0020 Implementation: Frontend Testing Toolchain

> **Related ADR**: [ADR-0020](../../adr/ADR-0020-frontend-technology-stack.md)  
> **Status**: Implementation Design  
> **Created**: 2026-02-04

---

## Overview

This document details the implementation of the frontend testing toolchain as defined in ADR-0020. It provides comprehensive configuration, CI enforcement gates, and best practices for React 19 + Next.js 15 applications.

## Goals

1. **Enforce quality standards** through mandatory CI gates
2. **Standardize testing patterns** across the frontend codebase
3. **Enable confident refactoring** with comprehensive test coverage
4. **Support Server Components** with appropriate testing strategies

---

## 1. Testing Toolchain

### Complete Stack

| Layer | Tool | Version | Purpose |
|-------|------|---------|---------|
| **Unit/Component Testing** | Vitest | 3.x | High-performance test runner, ESM-native |
| **Component Interaction** | React Testing Library | 16.x | User-centric component testing |
| **Browser Environment** | Vitest Browser Mode | 3.x | Real browser testing for visual components |
| **E2E Testing** | Playwright | 1.5x | Cross-browser automation |
| **Mocking** | MSW (Mock Service Worker) | 2.x | API mocking for integration tests |
| **Coverage** | v8 (via Vitest) | - | Native V8 coverage, fastest option |

### Testing Pyramid

Follow the industry-standard testing distribution:

| Test Type | Coverage | Purpose |
|-----------|----------|---------|
| **Unit Tests** | ~80% | Individual functions, hooks, utilities |
| **Integration Tests** | ~15% | Component interactions, API integration |
| **E2E Tests** | ~5% | Critical user journeys, full workflows |

---

## 2. Vitest Configuration

### Installation

```bash
npm install -D vitest @vitejs/plugin-react jsdom
npm install -D @testing-library/react @testing-library/jest-dom @testing-library/user-event
npm install -D @vitest/coverage-v8
npm install -D msw
```

### Configuration File

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import tsconfigPaths from 'vite-tsconfig-paths';

export default defineConfig({
  plugins: [react(), tsconfigPaths()],
  test: {
    // Environment
    environment: 'jsdom',
    globals: true,
    
    // Setup files
    setupFiles: ['./tests/setup.ts'],
    
    // Include patterns
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    
    // Coverage configuration (CI enforcement)
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/**/*.d.ts',
        'src/**/*.test.{ts,tsx}',
        'src/**/*.spec.{ts,tsx}',
        'src/**/index.ts',        // Barrel files
        'src/types/**',           // Type definitions
        'src/**/*.stories.tsx',   // Storybook stories
      ],
      // CI enforcement thresholds (MANDATORY)
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 75,
        statements: 80,
      },
    },
    
    // Performance
    pool: 'threads',
    poolOptions: {
      threads: {
        singleThread: false,
      },
    },
    
    // Timeout
    testTimeout: 10000,
    hookTimeout: 10000,
  },
});
```

### Setup File

```typescript
// tests/setup.ts
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, beforeAll, afterAll } from 'vitest';

// Automatic cleanup after each test
afterEach(() => {
  cleanup();
});

// MSW setup for API mocking
import { setupServer } from 'msw/node';
import { handlers } from './mocks/handlers';

export const server = setupServer(...handlers);

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```

---

## 3. React Testing Library Best Practices

### Query Priority

Use queries in this priority order for accessibility:

| Priority | Query | When to Use |
|----------|-------|-------------|
| 1 | `getByRole` | Interactive elements (buttons, inputs, links) |
| 2 | `getByLabelText` | Form elements with labels |
| 3 | `getByPlaceholderText` | Inputs with placeholders |
| 4 | `getByText` | Non-interactive elements with static text |
| 5 | `getByTestId` | Last resort, for complex/dynamic elements |

### Example: Component Test

```typescript
// src/components/ApprovalCard.test.tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { ApprovalCard } from './ApprovalCard';

describe('ApprovalCard', () => {
  const mockApproval = {
    id: 'req-001',
    vmName: 'dev-shop-redis-01',
    requestedBy: 'alice',
    status: 'pending',
  };

  it('displays approval request information', () => {
    render(<ApprovalCard approval={mockApproval} />);
    
    expect(screen.getByRole('heading', { name: /dev-shop-redis-01/i })).toBeInTheDocument();
    expect(screen.getByText(/requested by alice/i)).toBeInTheDocument();
    expect(screen.getByText(/pending/i)).toBeInTheDocument();
  });

  it('calls onApprove when approve button is clicked', async () => {
    const user = userEvent.setup();
    const onApprove = vi.fn();
    
    render(<ApprovalCard approval={mockApproval} onApprove={onApprove} />);
    
    await user.click(screen.getByRole('button', { name: /approve/i }));
    
    expect(onApprove).toHaveBeenCalledWith('req-001');
  });

  it('disables actions when status is not pending', () => {
    render(<ApprovalCard approval={{ ...mockApproval, status: 'approved' }} />);
    
    expect(screen.getByRole('button', { name: /approve/i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /reject/i })).toBeDisabled();
  });
});
```

### Example: Hook Test

```typescript
// src/hooks/useVMList.test.ts
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect } from 'vitest';
import { useVMList } from './useVMList';

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return ({ children }) => (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  );
};

describe('useVMList', () => {
  it('fetches VM list for a service', async () => {
    const { result } = renderHook(
      () => useVMList('service-001'),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toHaveLength(3);
    expect(result.current.data[0].name).toBe('dev-shop-redis-01');
  });

  it('handles error state', async () => {
    // MSW handler returns error for this service ID
    const { result } = renderHook(
      () => useVMList('error-service'),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error?.message).toContain('Service not found');
  });
});
```

---

## 4. Server Component Testing Strategy

### Challenge

React 19 Server Components execute on the server and send only final HTML to the client. Traditional unit testing with JSDOM cannot adequately test them.

### Recommended Approach

| Component Type | Testing Strategy | Tool |
|----------------|------------------|------|
| **Server Components** | E2E or Integration | Playwright + Next.js |
| **Client Components** | Unit + Integration | Vitest + RTL |
| **Shared Logic** | Unit | Vitest |

### Server Component Integration Test

```typescript
// tests/integration/system-list.test.ts
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { SystemListPage } from '@/app/(dashboard)/systems/page';

// Mock the data fetching
vi.mock('@/lib/api', () => ({
  getSystems: vi.fn().mockResolvedValue([
    { id: 'sys-001', name: 'shop', description: 'E-commerce system' },
    { id: 'sys-002', name: 'payment', description: 'Payment gateway' },
  ]),
}));

describe('SystemListPage (Server Component)', () => {
  it('renders system list from server data', async () => {
    // For Server Components, we need to await the component
    const Component = await SystemListPage();
    const { container } = render(Component);
    
    expect(container).toHaveTextContent('shop');
    expect(container).toHaveTextContent('payment');
  });
});
```

### Alternative: E2E for Server Components (Preferred)

```typescript
// tests/e2e/systems.spec.ts
import { test, expect } from '@playwright/test';

test.describe('System List Page', () => {
  test('displays system list from API', async ({ page }) => {
    await page.goto('/systems');
    
    // Wait for server-rendered content
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
    
    // Verify data rendered from Server Component
    await expect(page.getByText('shop')).toBeVisible();
    await expect(page.getByText('E-commerce system')).toBeVisible();
  });
});
```

---

## 5. Playwright Configuration

### Installation

```bash
npm init playwright@latest
```

### Configuration File

```typescript
// playwright.config.ts
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? 'github' : 'html',
  
  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
  ],

  // Start Next.js dev server before running tests
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    timeout: 120 * 1000,
  },
});
```

### E2E Test Example

```typescript
// tests/e2e/approval-workflow.spec.ts
import { test, expect } from '@playwright/test';

test.describe('Approval Workflow', () => {
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Login' }).click();
    await expect(page).toHaveURL('/dashboard');
  });

  test('admin can approve a pending VM request', async ({ page }) => {
    // Navigate to approvals
    await page.getByRole('link', { name: 'Approvals' }).click();
    await expect(page.getByRole('heading', { name: 'Pending Approvals' })).toBeVisible();
    
    // Find and approve the first pending request
    const firstRequest = page.getByTestId('approval-card').first();
    await firstRequest.getByRole('button', { name: 'Approve' }).click();
    
    // Confirm in modal
    await page.getByRole('dialog').getByRole('button', { name: 'Confirm' }).click();
    
    // Verify success
    await expect(page.getByText('Request approved successfully')).toBeVisible();
  });

  test('user must type VM name to delete production VM', async ({ page }) => {
    await page.goto('/vms');
    
    // Find a production VM
    const prodVM = page.locator('[data-environment="prod"]').first();
    await prodVM.getByRole('button', { name: 'Delete' }).click();
    
    // Modal requires typing VM name
    const modal = page.getByRole('dialog');
    await expect(modal.getByText('Type the VM name to confirm')).toBeVisible();
    
    // Try to delete without typing - should be disabled
    await expect(modal.getByRole('button', { name: 'Delete' })).toBeDisabled();
    
    // Type the VM name
    const vmName = await prodVM.getByTestId('vm-name').textContent();
    await modal.getByRole('textbox').fill(vmName!);
    
    // Now delete should be enabled
    await expect(modal.getByRole('button', { name: 'Delete' })).toBeEnabled();
  });
});
```

---

## 6. CI Configuration

### GitHub Actions Workflow

```yaml
# .github/workflows/frontend-tests.yml
name: Frontend Tests

on:
  push:
    branches: [main]
    paths:
      - 'web/**'
  pull_request:
    paths:
      - 'web/**'

jobs:
  unit-tests:
    name: Unit & Integration Tests
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '22'
          cache: 'npm'
          cache-dependency-path: web/package-lock.json
      
      - name: Install dependencies
        run: npm ci
      
      - name: Run linting
        run: npm run lint
      
      - name: Run type check
        run: npm run typecheck
      
      - name: Run unit tests with coverage
        run: npm run test:coverage
        
      - name: Upload coverage report
        uses: codecov/codecov-action@v4
        with:
          files: web/coverage/lcov.info
          fail_ci_if_error: true

  e2e-tests:
    name: E2E Tests
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '22'
          cache: 'npm'
          cache-dependency-path: web/package-lock.json
      
      - name: Install dependencies
        run: npm ci
      
      - name: Install Playwright browsers
        run: npx playwright install --with-deps chromium
      
      - name: Run E2E tests
        run: npx playwright test --project=chromium
      
      - name: Upload test results
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: playwright-report
          path: web/playwright-report/
          retention-days: 7
```

### Package.json Scripts

```json
{
  "scripts": {
    "test": "vitest",
    "test:run": "vitest run",
    "test:coverage": "vitest run --coverage",
    "test:ui": "vitest --ui",
    "test:e2e": "playwright test",
    "test:e2e:ui": "playwright test --ui",
    "typecheck": "tsc --noEmit",
    "lint": "eslint . --ext .ts,.tsx"
  }
}
```

### CI Quality Gates (Mandatory)

| Gate | Tool | Fail Condition |
|------|------|----------------|
| **Type Safety** | TypeScript | Any error with `strict: true` |
| **Lint** | ESLint | Any error |
| **Unit Tests** | Vitest | Any test failure |
| **Coverage** | v8 | **< 80% lines/functions/statements, < 75% branches** |
| **E2E Tests** | Playwright | Any critical path failure |

> ⚠️ **MANDATORY**: Coverage below thresholds will **BLOCK** the PR from merging.

---

## 7. MSW (Mock Service Worker) Setup

### Handlers Definition

```typescript
// tests/mocks/handlers.ts
import { http, HttpResponse } from 'msw';

export const handlers = [
  // List VMs
  http.get('/api/v1/services/:serviceId/vms', ({ params }) => {
    if (params.serviceId === 'error-service') {
      return HttpResponse.json(
        { code: 'NOT_FOUND', message: 'Service not found' },
        { status: 404 }
      );
    }
    return HttpResponse.json([
      { id: 'vm-001', name: 'dev-shop-redis-01', status: 'running' },
      { id: 'vm-002', name: 'dev-shop-redis-02', status: 'stopped' },
      { id: 'vm-003', name: 'dev-shop-redis-03', status: 'running' },
    ]);
  }),

  // Create VM
  http.post('/api/v1/vms', async ({ request }) => {
    const body = await request.json();
    return HttpResponse.json(
      { id: 'vm-new', ...body, status: 'pending_approval' },
      { status: 201 }
    );
  }),

  // Approve request
  http.post('/api/v1/approvals/:id/approve', () => {
    return HttpResponse.json({ success: true });
  }),
];
```

---

## 8. Implementation Checklist

### Phase 1: Foundation

- [ ] Install testing dependencies (Vitest, RTL, Playwright)
- [ ] Create `vitest.config.ts` with coverage thresholds
- [ ] Create `tests/setup.ts` with cleanup and MSW
- [ ] Create `tests/mocks/handlers.ts` for API mocking

### Phase 2: CI Integration

- [ ] Create `.github/workflows/frontend-tests.yml`
- [ ] Add coverage reporting to Codecov
- [ ] Add E2E tests with Playwright
- [ ] Configure required status checks

### Phase 3: Documentation

- [ ] Update `web/README.md` with testing instructions
- [ ] Create `web/TESTING.md` with patterns and examples
- [ ] Add testing section to `CONTRIBUTING.md`

---

## References

- [Vitest Documentation](https://vitest.dev/)
- [React Testing Library](https://testing-library.com/docs/react-testing-library/intro/)
- [Playwright Documentation](https://playwright.dev/)
- [MSW (Mock Service Worker)](https://mswjs.io/)
- [Next.js Testing](https://nextjs.org/docs/app/building-your-application/testing)
- [Testing Playground](https://testing-playground.com/) - Find best queries for elements

---

_End of ADR-0020 Implementation: Frontend Testing Toolchain_

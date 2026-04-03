import { act, renderHook, waitFor } from "@testing-library/react";
import type { TFunction } from "i18next";
import { beforeEach, describe, expect, it, vi } from "vitest";

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  messageSuccessMock,
  messageErrorMock,
  createFormState,
  editFormState,
  auxFormState,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  createFormState: {
    validateFields: vi.fn(),
    getFieldsValue: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  editFormState: {
    validateFields: vi.fn(),
    getFieldsValue: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  auxFormState: {
    validateFields: vi.fn(),
    getFieldsValue: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
}));

vi.mock("antd", () => ({
  App: {
    useApp: () => ({
      message: {
        success: messageSuccessMock,
        error: messageErrorMock,
      },
    }),
  },
  Form: {
    useForm: (...args: unknown[]) => useFormMock(...args),
  },
}));

vi.mock("@/hooks/useApiQuery", () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
  useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

import { useAdminAuthProvidersController } from "./useAdminAuthProvidersController";

describe("useAdminAuthProvidersController", () => {
  const t = ((key: string, options?: { defaultValue?: string }) =>
    options?.defaultValue ?? key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      const slot = formCall % 7;
      formCall += 1;
      if (slot === 0) return [createFormState];
      if (slot === 1) return [editFormState];
      return [auxFormState];
    });

    createFormState.validateFields.mockResolvedValue({
      name: "corp-oidc",
      auth_type: "oidc",
      enabled: true,
      sort_order: 10,
    });
    createFormState.getFieldsValue.mockReturnValue({
      config: { issuer: "https://idp.example.com" },
    });
    editFormState.getFieldsValue.mockReturnValue({});
    auxFormState.validateFields.mockResolvedValue({});
    auxFormState.getFieldsValue.mockReturnValue({});

    useApiGetMock.mockReturnValue({
      data: { items: [] },
      isLoading: false,
      refetch: vi.fn(),
    });
  });

  it("submits create payload with schema-driven config object", async () => {
    const createMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall === 1) return { mutate: createMutate, isPending: false };
      return { mutate: vi.fn(), isPending: false };
    });

    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
      await result.current.submitCreate();
    });

    expect(createMutate).toHaveBeenCalledWith({
      name: "corp-oidc",
      auth_type: "oidc",
      enabled: true,
      sort_order: 10,
      config: { issuer: "https://idp.example.com" },
    });
  });

  it("omits config when no schema-driven values are provided", async () => {
    const createMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall === 1) return { mutate: createMutate, isPending: false };
      return { mutate: vi.fn(), isPending: false };
    });

    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    createFormState.validateFields.mockResolvedValueOnce({
      name: "configless-provider",
      auth_type: "oidc",
      enabled: true,
    });
    createFormState.getFieldsValue.mockReturnValueOnce({});

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
      await result.current.submitCreate();
    });

    expect(createMutate).toHaveBeenCalledWith({
      name: "configless-provider",
      auth_type: "oidc",
      enabled: true,
      sort_order: undefined,
      config: undefined,
    });
  });

  it("uses backend-discovered provider types when opening create modal", async () => {
    useApiGetMock.mockImplementation((queryKey?: unknown) => {
      if (
        Array.isArray(queryKey) &&
        queryKey[0] === "admin-auth-provider-types"
      ) {
        return {
          data: {
            items: [
              {
                type: "custom-sso",
                display_name: "Custom SSO",
                built_in: false,
              },
            ],
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      };
    });

    useApiMutationMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
    });

    await waitFor(() =>
      expect(createFormState.setFieldsValue).toHaveBeenCalledWith(
        expect.objectContaining({
          auth_type: "custom-sso",
          enabled: true,
          sort_order: 0,
        }),
      ),
    );
  });

  it("parses schema-driven object fields from JSON text before submit", async () => {
    const createMutate = vi.fn();

    useApiGetMock
      .mockImplementationOnce(() => ({
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      }))
      .mockImplementationOnce(() => ({
        data: {
          items: [
            {
              type: "oidc",
              display_name: "OIDC",
              built_in: true,
              config_schema: {
                type: "object",
                properties: {
                  issuer_url: { type: "string" },
                  claims_mapping: { type: "object" },
                },
              },
            },
          ],
        },
        isLoading: false,
        refetch: vi.fn(),
      }))
      .mockImplementation(() => ({
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      }));

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall === 1) return { mutate: createMutate, isPending: false };
      return { mutate: vi.fn(), isPending: false };
    });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    createFormState.validateFields.mockResolvedValueOnce({
      name: "corp-oidc",
      auth_type: "oidc",
      enabled: true,
    });
    createFormState.getFieldsValue.mockReturnValueOnce({
      config: {
        issuer_url: "https://issuer.example.com",
        claims_mapping: '{"email":"mail","name":"cn"}',
      },
    });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
      await result.current.submitCreate();
    });

    expect(createMutate).toHaveBeenCalledWith({
      name: "corp-oidc",
      auth_type: "oidc",
      enabled: true,
      sort_order: undefined,
      config: {
        issuer_url: "https://issuer.example.com",
        claims_mapping: {
          email: "mail",
          name: "cn",
        },
      },
    });
  });

  it("hydrates edit form config values when opening the edit modal", async () => {
    useApiGetMock.mockImplementation((queryKey?: unknown) => {
      if (
        Array.isArray(queryKey) &&
        queryKey[0] === "admin-auth-provider-types"
      ) {
        return {
          data: {
            items: [
              {
                type: "generic",
                display_name: "Corp SSO",
                built_in: false,
                config_schema: {
                  type: "object",
                  properties: {
                    login_entry_url: { type: "string" },
                    callback_param_name: {
                      type: "string",
                      default: "redirect_uri",
                    },
                    upstream_token_transport: {
                      type: "string",
                      default: "query",
                    },
                    userinfo_endpoint: { type: "string" },
                  },
                },
              },
            ],
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      };
    });

    useApiMutationMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openEditModal({
        id: "provider-1",
        name: "corp-sso",
        auth_type: "generic",
        enabled: true,
        sort_order: 10,
        config: {
          login_entry_url: "https://portal.example.com/login",
          userinfo_endpoint: "https://portal.example.com/api/userinfo",
        },
      } as never);
    });

    await waitFor(() =>
      expect(editFormState.setFieldsValue).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "corp-sso",
          enabled: true,
          sort_order: 10,
          config: {
            login_entry_url: "https://portal.example.com/login",
            callback_param_name: "redirect_uri",
            upstream_token_transport: "query",
            userinfo_endpoint: "https://portal.example.com/api/userinfo",
          },
        }),
      ),
    );
  });

  it("recommends department mapping defaults when department sample data is present", async () => {
    useApiGetMock.mockImplementation((queryKey?: unknown) => {
      if (
        Array.isArray(queryKey) &&
        queryKey[0] === "admin-auth-provider-sample"
      ) {
        return {
          data: {
            fields: [
              {
                field: "department",
                value_type: "string",
                distinct_count: 1,
                sample: ["Engineering"],
              },
            ],
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      };
    });

    useApiMutationMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openMappingModal({
        id: "provider-1",
        name: "corp-directory-enrichment",
        auth_type: "generic",
        enabled: true,
      } as never);
    });

    expect(result.current.recommendedCohortDefaults).toEqual({
      cohortKind: "department",
      sourceField: "department",
      reason: "sample_department",
    });
  });

  it("hydrates directory request defaults from the scheduled enrichment plan", async () => {
    useApiGetMock.mockImplementation((queryKey?: unknown) => {
      if (
        Array.isArray(queryKey) &&
        queryKey[0] === "admin-auth-provider-directory-descriptor"
      ) {
        return {
          data: {
            request_schema: {
              type: "object",
              properties: {
                department_names: { type: "array" },
                include_nested: { type: "boolean", default: true },
              },
            },
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (
        Array.isArray(queryKey) &&
        queryKey[0] === "admin-auth-provider-directory-schedule"
      ) {
        return {
          data: {
            supported: true,
            enabled: true,
            provider_request: {
              department_names: ["Engineering", "Finance"],
              include_nested: true,
            },
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      };
    });

    useApiMutationMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openMappingModal({
        id: "provider-1",
        name: "corp-directory-enrichment",
        auth_type: "generic",
        enabled: true,
      } as never);
    });

    await waitFor(() =>
      expect(auxFormState.setFieldsValue).toHaveBeenCalledWith(
        expect.objectContaining({
          provider_request: {
            department_names: ["Engineering", "Finance"],
            include_nested: true,
          },
        }),
      ),
    );
  });

  it("accepts manual cohort sync values from tag-style selection inputs", async () => {
    const syncCohortsMutate = vi.fn();

    useApiMutationMock.mockReturnValue({
      mutate: syncCohortsMutate,
      isPending: false,
    });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    auxFormState.validateFields.mockResolvedValueOnce({
      cohort_kind: "group",
      source_field: "groups",
      cohorts_text: ["ops-team", "platform-admin"],
    });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openMappingModal({
        id: "provider-1",
        name: "corp-sso",
        auth_type: "generic",
        enabled: true,
      } as never);
    });

    await act(async () => {
      await result.current.submitSyncCohorts();
    });

    expect(syncCohortsMutate).toHaveBeenCalledWith({
      providerId: "provider-1",
      body: {
        cohort_kind: "group",
        source_field: "groups",
        cohorts: ["ops-team", "platform-admin"],
      },
    });
  });
});

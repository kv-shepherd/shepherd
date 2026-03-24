import { act, renderHook } from "@testing-library/react";
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
  Form: {
    useForm: (...args: unknown[]) => useFormMock(...args),
  },
  message: {
    useMessage: () => [
      {
        success: messageSuccessMock,
        error: messageErrorMock,
      },
      null,
    ],
  },
}));

vi.mock("@/hooks/useApiQuery", () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
  useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

import { useAdminAuthProvidersController } from "./useAdminAuthProvidersController";

describe("useAdminAuthProvidersController", () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      if (formCall === 1) return [createFormState];
      if (formCall === 2) return [editFormState];
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
    useApiGetMock
      .mockImplementationOnce(() => ({
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      }))
      .mockImplementationOnce(() => ({
        data: {
          items: [
            { type: "custom-sso", display_name: "Custom SSO", built_in: false },
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

    useApiMutationMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
    });

    expect(createFormState.setFieldsValue).toHaveBeenCalledWith(
      expect.objectContaining({
        auth_type: "custom-sso",
        enabled: true,
        sort_order: 0,
      }),
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
});

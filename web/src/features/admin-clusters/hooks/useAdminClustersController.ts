"use client";

import { Form, message } from "antd";
import type { TFunction } from "i18next";
import { useRef, useState } from "react";

import { useApiGet, useApiMutation } from "@/hooks/useApiQuery";
import { api } from "@/lib/api/client";
import type { components } from "@/types/api.gen";

import type {
  Cluster,
  ClusterCreateRequest,
  ClusterList,
  ClusterPolicy,
  ClusterPolicyUpsertRequest,
} from "../types";

interface UseAdminClustersControllerArgs {
  t: TFunction;
}

type NamespaceRegistryList = components["schemas"]["NamespaceRegistryList"];

export function useAdminClustersController({
  t,
}: UseAdminClustersControllerArgs) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const [createOpen, setCreateOpen] = useState(false);
  const [envModalOpen, setEnvModalOpen] = useState(false);
  const [policyModalOpen, setPolicyModalOpen] = useState(false);
  const [policyLoading, setPolicyLoading] = useState(false);
  const [selectedClusterPolicyExists, setSelectedClusterPolicyExists] =
    useState(false);
  const [selectedClusterId, setSelectedClusterId] = useState<string>("");
  const [selectedClusterName, setSelectedClusterName] = useState<string>("");
  const [selectedClusterEnv, setSelectedClusterEnv] = useState<"test" | "prod">(
    "test",
  );
  const [selectedClusterStorageClasses, setSelectedClusterStorageClasses] =
    useState<string[]>([]);
  const selectedClusterIdRef = useRef("");
  const [form] = Form.useForm<ClusterCreateRequest>();
  const [envForm] = Form.useForm<{ environment: "test" | "prod" }>();
  const [policyForm] = Form.useForm<ClusterPolicyUpsertRequest>();

  const clusterListQuery = useApiGet<ClusterList>(["admin-clusters"], () =>
    api.GET("/admin/clusters"),
  );

  const policyNamespaceQuery = useApiGet<NamespaceRegistryList>(
    ["admin-cluster-policy-namespaces", policyModalOpen, selectedClusterEnv],
    () =>
      api.GET("/admin/namespaces", {
        params: {
          query: {
            page: 1,
            per_page: 200,
            environment: selectedClusterEnv,
          },
        },
      }),
    {
      enabled: policyModalOpen && selectedClusterId !== "",
    },
  );

  const createMutation = useApiMutation<ClusterCreateRequest, Cluster>(
    (req) => api.POST("/admin/clusters", { body: req }),
    {
      invalidateKeys: [["admin-clusters"]],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
        closeCreateModal();
      },
      onError: (err) =>
        messageApi.error(err.message || t("common:message.error")),
    },
  );

  const updateEnvironmentMutation = useApiMutation<
    { clusterId: string; environment: "test" | "prod" },
    Cluster
  >(
    ({ clusterId, environment }) =>
      api.PUT("/admin/clusters/{cluster_id}/environment", {
        params: { path: { cluster_id: clusterId } },
        body: { environment },
      }),
    {
      invalidateKeys: [["admin-clusters"]],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) =>
        messageApi.error(err.message || t("common:message.error")),
    },
  );

  const upsertPolicyMutation = useApiMutation<
    { clusterId: string; body: ClusterPolicyUpsertRequest },
    ClusterPolicy
  >(
    ({ clusterId, body }) =>
      api.PUT("/admin/clusters/{cluster_id}/policy", {
        params: { path: { cluster_id: clusterId } },
        body,
      }),
    {
      invalidateKeys: [["admin-clusters"]],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) =>
        messageApi.error(err.message || t("common:message.error")),
    },
  );

  const openCreateModal = () => {
    setCreateOpen(true);
  };

  const closeCreateModal = () => {
    setCreateOpen(false);
    form.resetFields();
  };

  const submitCreate = async () => {
    const values = await form.validateFields();
    createMutation.mutate(values);
  };

  const updateEnvironment = (
    clusterId: string,
    environment: "test" | "prod",
  ) => {
    updateEnvironmentMutation.mutate({ clusterId, environment });
  };

  const openEnvModal = (clusterId: string, currentEnv: "test" | "prod") => {
    setSelectedClusterId(clusterId);
    setSelectedClusterEnv(currentEnv);
    envForm.setFieldsValue({ environment: currentEnv });
    setEnvModalOpen(true);
  };

  const closeEnvModal = () => {
    setEnvModalOpen(false);
    setSelectedClusterId("");
    envForm.resetFields();
  };

  const submitEnvUpdate = async () => {
    const values = await envForm.validateFields();
    await updateEnvironmentMutation.mutateAsync({
      clusterId: selectedClusterId,
      environment: values.environment,
    });
    closeEnvModal();
  };

  const openPolicyModal = async (cluster: Cluster) => {
    selectedClusterIdRef.current = cluster.id;
    setSelectedClusterId(cluster.id);
    setSelectedClusterName(cluster.display_name ?? cluster.name ?? cluster.id);
    setSelectedClusterEnv(cluster.environment ?? "test");
    setSelectedClusterStorageClasses(cluster.storage_classes ?? []);
    setPolicyModalOpen(true);
    setPolicyLoading(true);

    const { data, error, response } = await api.GET(
      "/admin/clusters/{cluster_id}/policy",
      {
        params: { path: { cluster_id: cluster.id } },
      },
    );

    if (error) {
      if (response.status === 404) {
        setSelectedClusterPolicyExists(false);
        policyForm.setFieldsValue(defaultClusterPolicyFormValues());
        setPolicyLoading(false);
        return;
      }
      messageApi.error(error.message || t("common:message.error"));
      closePolicyModal();
      setPolicyLoading(false);
      return;
    }

    setSelectedClusterPolicyExists(true);
    policyForm.setFieldsValue(clusterPolicyToFormValues(data));
    setPolicyLoading(false);
  };

  const closePolicyModal = () => {
    setPolicyModalOpen(false);
    setPolicyLoading(false);
    selectedClusterIdRef.current = "";
    setSelectedClusterId("");
    setSelectedClusterName("");
    setSelectedClusterEnv("test");
    setSelectedClusterPolicyExists(false);
    setSelectedClusterStorageClasses([]);
    policyForm.resetFields();
  };

  const submitPolicyUpdate = async () => {
    const values = await policyForm.validateFields();
    await upsertPolicyMutation.mutateAsync({
      clusterId: selectedClusterIdRef.current,
      body: normalizePolicyFormValues(values),
    });
    closePolicyModal();
  };

  return {
    messageContextHolder,
    createOpen,
    form,
    data: clusterListQuery.data,
    isLoading: clusterListQuery.isLoading,
    refetch: clusterListQuery.refetch,
    openCreateModal,
    closeCreateModal,
    submitCreate,
    updateEnvironment,
    createPending: createMutation.isPending,
    updateEnvironmentPending: updateEnvironmentMutation.isPending,
    envModalOpen,
    selectedClusterId,
    selectedClusterName,
    selectedClusterEnv,
    selectedClusterStorageClasses,
    selectedClusterNamespaceOptions: (policyNamespaceQuery.data?.items ?? [])
      .filter((item) => item.name && item.enabled !== false)
      .map((item) => item.name)
      .sort((a, b) => a.localeCompare(b)),
    namespaceSuggestionsLoading: policyNamespaceQuery.isLoading,
    envForm,
    openEnvModal,
    closeEnvModal,
    submitEnvUpdate,
    policyModalOpen,
    policyLoading,
    selectedClusterPolicyExists,
    policyForm,
    openPolicyModal,
    closePolicyModal,
    submitPolicyUpdate,
    upsertPolicyPending: upsertPolicyMutation.isPending,
  };
}

function defaultClusterPolicyFormValues(): ClusterPolicyUpsertRequest {
  return {
    allow_cpu_overcommit: true,
    allow_memory_overcommit: true,
    allow_dedicated_cpu: false,
    allow_gpu: false,
    allow_sriov: false,
    allow_hugepages: false,
    allowed_hugepages_sizes: [],
    allow_cdi_clone: true,
    allowed_clone_source_namespaces: [],
    allowed_storage_classes: [],
  };
}

function clusterPolicyToFormValues(
  policy: ClusterPolicy,
): ClusterPolicyUpsertRequest {
  return {
    allow_cpu_overcommit: policy.allow_cpu_overcommit,
    allow_memory_overcommit: policy.allow_memory_overcommit,
    allow_dedicated_cpu: policy.allow_dedicated_cpu,
    allow_gpu: policy.allow_gpu,
    allow_sriov: policy.allow_sriov,
    allow_hugepages: policy.allow_hugepages,
    allowed_hugepages_sizes: policy.allowed_hugepages_sizes ?? [],
    allow_cdi_clone: policy.allow_cdi_clone,
    allowed_clone_source_namespaces:
      policy.allowed_clone_source_namespaces ?? [],
    allowed_storage_classes: policy.allowed_storage_classes ?? [],
  };
}

function normalizePolicyFormValues(
  values: ClusterPolicyUpsertRequest,
): ClusterPolicyUpsertRequest {
  return {
    ...values,
    allowed_hugepages_sizes: values.allow_hugepages
      ? (values.allowed_hugepages_sizes ?? [])
      : [],
    allowed_clone_source_namespaces: values.allow_cdi_clone
      ? (values.allowed_clone_source_namespaces ?? [])
      : [],
    allowed_storage_classes: values.allowed_storage_classes ?? [],
  };
}

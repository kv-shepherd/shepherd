package service // want `core auth-provider file must stay provider-neutral; found provider-specific fragment`

func badBranch(providerType string) string {
	switch providerType {
	case "wecom":
		return "department"
	default:
		return "ok"
	}
}

package provider

import "testing"

type testAuthCallbackOriginDescriber struct{}

func (testAuthCallbackOriginDescriber) AllowedCallbackOrigins(config map[string]interface{}) []string {
	origin, _ := config["origin"].(string)
	if origin == "" {
		return nil
	}
	return []string{origin}
}

func TestAuthCallbackOriginDescriberAlias(t *testing.T) {
	t.Parallel()

	var describer AuthCallbackOriginDescriber = testAuthCallbackOriginDescriber{}
	got := describer.AllowedCallbackOrigins(map[string]interface{}{
		"origin": "https://login.example.com",
	})
	if len(got) != 1 || got[0] != "https://login.example.com" {
		t.Fatalf("AllowedCallbackOrigins() = %#v, want login origin", got)
	}
}

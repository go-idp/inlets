package main

import (
	"testing"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-idp/inlets/internal/server/types"
)

func TestCreateGetTokenFunctionWithRef_CredentialsOpaqueChildStillGetsTunnels(t *testing.T) {
	configRef := config.NewRef(&config.FileConfig{
		Token: "server-token",
		Clients: []config.ClientConfig{
			{
				ClientID:     "client1",
				ClientSecret: "secret1",
				Tunnels: []client.TunnelSpec{
					{
						Name:      "web",
						Type:      "http",
						Upstream:  "127.0.0.1:9000",
						SubDomain: "myapp",
						Auth: &client.HTTPIncomingAuthRule{
							Enable: true,
							Users: []client.HTTPTunnelAuth{
								{Type: "bearer", Token: "service-token"},
							},
						},
					},
				},
			},
		},
	})

	getToken := config.CreateGetToken(configRef, ServerVersion)
	res, err := getToken(types.AuthTypeCredentials, "client1", &types.GetTokenOptions{
		Type:        types.TunnelTypeHTTP,
		OpaqueChild: true,
	})
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if res == nil || res.Config == nil {
		t.Fatalf("expected token response with config")
	}
	if len(res.Config.Tunnels) != 1 {
		t.Fatalf("expected opaque child to still receive tunnel list for server-side auth matching, got %d", len(res.Config.Tunnels))
	}
}

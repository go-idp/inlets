package config

import (
	"fmt"
	"strings"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/types"
)

// CreateGetToken returns a GetToken function backed by ref.
func CreateGetToken(ref *Ref, serverVersion string) types.GetToken {
	return func(authType types.AuthType, clientId string, options *types.GetTokenOptions) (*types.TokenResponse, error) {
		if authType == types.AuthTypePublic {
			if options != nil && options.Type != types.TunnelTypeHTTP {
				return nil, fmt.Errorf("public auth is only allowed for http tunnel")
			}
			return &types.TokenResponse{
				AuthType: types.AuthTypePublic,
				Token:    "public",
			}, nil
		}

		ref.mu.RLock()
		defer ref.mu.RUnlock()

		cfg := ref.config
		if cfg == nil {
			return nil, fmt.Errorf("config file is required")
		}

		if authType == types.AuthTypeCredentials {
			for _, clientCfg := range cfg.Clients {
				if clientCfg.ClientID == clientId {
					var clientConfig *client.Config
					if clientCfg.Config != nil {
						clientConfig = &client.Config{
							Version:                clientCfg.Config.Version,
							Notification:         clientCfg.Config.Notification,
							NegotiatedCapabilities: clientCfg.Config.NegotiatedCapabilities,
						}
						if clientConfig.Version == "" {
							clientConfig.Version = serverVersion
						}
					} else {
						clientConfig = &client.Config{Version: serverVersion}
					}
					if len(clientCfg.Tunnels) > 0 {
						tunnelsCopy := make([]client.TunnelSpec, len(clientCfg.Tunnels))
						copy(tunnelsCopy, clientCfg.Tunnels)
						clientConfig.Tunnels = tunnelsCopy
					}
					return &types.TokenResponse{
						AuthType: types.AuthTypeCredentials,
						Token:    clientCfg.ClientSecret,
						Config:   clientConfig,
					}, nil
				}
			}
			return nil, fmt.Errorf("client not found: %s", clientId)
		}

		configToken := cfg.Token
		if configToken == "" {
			return nil, fmt.Errorf("token is required for token authentication")
		}
		return &types.TokenResponse{
			AuthType: types.AuthTypeToken,
			Token:    configToken,
			Config: &client.Config{
				Version: serverVersion,
				Tunnels: collectHTTPAuthTunnels(cfg.Clients),
			},
		}, nil
	}
}

func collectHTTPAuthTunnels(clients []ClientConfig) []client.TunnelSpec {
	if len(clients) == 0 {
		return nil
	}
	var out []client.TunnelSpec
	for i := range clients {
		for j := range clients[i].Tunnels {
			t := clients[i].Tunnels[j]
			if !strings.EqualFold(strings.TrimSpace(t.Type), "http") {
				continue
			}
			hasAuth := (t.Auth != nil && t.Auth.Enable && len(t.Auth.Users) > 0) || len(t.Auths) > 0
			if !hasAuth {
				continue
			}
			cp := t
			if t.Auth != nil {
				authCopy := *t.Auth
				if len(t.Auth.Users) > 0 {
					users := make([]client.HTTPTunnelAuth, len(t.Auth.Users))
					copy(users, t.Auth.Users)
					authCopy.Users = users
				}
				cp.Auth = &authCopy
			}
			if len(t.Auths) > 0 {
				auths := make([]client.HTTPTunnelAuth, len(t.Auths))
				copy(auths, t.Auths)
				cp.Auths = auths
			}
			out = append(out, cp)
		}
	}
	return out
}

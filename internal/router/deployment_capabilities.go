package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/sandbox"
)

func deploymentCapabilitiesFromRouter(params RouterParams) handler.DeploymentCapabilitiesData {
	return handler.BuildDeploymentCapabilities(handler.Edition, handler.DeploymentFeatureAvailability{
		Organizations: params.Agent.OrganizationHandler != nil,
		Agents:        params.Agent.CustomAgentHandler != nil,
		IM:            params.Integration.IMHandler != nil,
		// Match RegisterEmbedChannelRoutes: management routes depend on handler only.
		Embed: params.Integration.EmbedChannelHandler != nil,
		// Match RegisterMCPEndpointRoutes / RegisterMCPServerRoutes.
		MCPServer: params.MCP.MCPEndpointHandler != nil && params.MCP.MCPServer != nil && params.MCP.MCPEndpointService != nil,
		API:       params.AuthTenant.TenantHandler != nil && params.AuthTenant.TenantAPIKeyService != nil,
		MCP: params.MCP.MCPServiceHandler != nil &&
			params.MCP.MCPCredentialsHandler != nil &&
			params.MCP.MCPOAuthHandler != nil,
		WebSearch: params.WebSearch.WebSearchHandler != nil &&
			params.WebSearch.WebSearchProviderHandler != nil &&
			params.WebSearch.WebSearchCredentialsHandler != nil,
		VectorStore:   params.Core.VectorStoreHandler != nil,
		Storage:       params.Core.StorageBackendHandler != nil,
		Sandbox:       params.Sandbox.SandboxConfigHandler != nil,
		SandboxDocker: sandbox.DockerBackendEnabled(),
		SandboxHost:   params.Sandbox.HostSandbox.Manager != nil,
		SandboxRemote: params.Sandbox.SandboxConfigHandler != nil && !params.Sandbox.HostSandbox.Desktop,
	})
}

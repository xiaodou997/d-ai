package console

import (
	"strings"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/application"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
)

const (
	consoleAgentTypeChat            = "chat"
	consoleAgentTypeImageGeneration = "image_generation"
	consoleAgentTypeImageEdit       = "image_edit"
)

func appAgentRecordToConsoleChatAgentDTO(row pgadapter.AppAgentRecord) consoleChatAgentDTO {
	primary := primaryAppPromptBinding(row)
	return consoleChatAgentDTO{
		ID:             row.ID,
		Name:           row.Name,
		Description:    row.Description,
		PublisherLabel: consoleAppPublisherLabel(row),
		Variables:      decodeStringSlice(primary.Variables),
	}
}

func appAgentRecordToConsoleChatAgentRuntime(row pgadapter.AppAgentRecord) consoleChatAgentRuntime {
	primary := primaryAppPromptBinding(row)
	bindings := runtimePromptBindings(row)
	return consoleChatAgentRuntime{
		OwnerType:      row.OwnerType,
		OwnerTenantID:  row.OwnerTenantID,
		OwnerUserID:    row.OwnerUserID,
		ID:             row.ID,
		Name:           row.Name,
		Description:    row.Description,
		GroupID:        row.GroupID,
		ModelCode:      row.ModelCode,
		TemplateText:   primary.TemplateText,
		Variables:      decodeStringSlice(primary.Variables),
		PromptStrategy: application.PromptStrategy(row.PromptStrategy),
		PromptBindings: bindings,
		DefaultOptions: decodeJSONObject(row.DefaultOptions),
		AgentType:      row.Capability,
	}
}

func appAgentRecordToConsoleImageAgentDTO(row pgadapter.AppAgentRecord) consoleImageAgentDTO {
	primary := primaryAppPromptBinding(row)
	cfg := application.ParseRuntimeConfig(application.AppTypeImageGenerationAgent, decodeJSONObject(row.DefaultOptions)).Image
	return consoleImageAgentDTO{
		ID:                       row.ID,
		Name:                     row.Name,
		Description:              row.Description,
		AgentType:                row.Capability,
		PublisherLabel:           consoleAppPublisherLabel(row),
		Variables:                decodeStringSlice(primary.Variables),
		DefaultOutputCount:       cfg.DefaultOutputCount,
		MaxOutputCount:           cfg.MaxOutputCount,
		AllowOutputCountOverride: cfg.AllowOutputCountOverride,
	}
}

func primaryAppPromptBinding(row pgadapter.AppAgentRecord) pgadapter.AppPromptBindingRecord {
	for _, binding := range row.PromptBindings {
		if binding.BindingRole == "primary" {
			return binding
		}
	}
	return pgadapter.AppPromptBindingRecord{}
}

func runtimePromptBindings(row pgadapter.AppAgentRecord) []application.RuntimePromptBinding {
	out := make([]application.RuntimePromptBinding, 0, len(row.PromptBindings))
	for _, binding := range row.PromptBindings {
		role := application.PromptBindingInputTemplate
		if binding.BindingRole == "primary" && row.Capability == consoleAgentTypeChat {
			role = application.PromptBindingSystem
		}
		out = append(out, application.RuntimePromptBinding{
			PromptID: binding.PromptID, PromptName: binding.PromptName,
			PromptRevision: int(binding.CurrentRevision), TemplateText: binding.TemplateText,
			Variables: decodeStringSlice(binding.Variables), Role: role,
			BindingOrder: int(binding.DisplayOrder),
		})
	}
	return out
}

func resolveConsoleAppPrompt(agent consoleChatAgentRuntime, input string, variables map[string]string) (application.ResolvedPrompt, error) {
	resolved, err := application.ResolvePrompt(agent.PromptStrategy, application.PromptResolveInput{
		Input: input, Variables: variables, Bindings: agent.PromptBindings,
	})
	if err != nil {
		return application.ResolvedPrompt{}, err
	}
	return resolved, nil
}

func consoleAppPublisherLabel(row pgadapter.AppAgentRecord) string {
	switch row.OwnerType {
	case "platform":
		return "平台"
	case "tenant":
		if strings.TrimSpace(row.OwnerTenantID) != "" {
			return row.OwnerTenantID
		}
		return "租户"
	case "user":
		return "我"
	default:
		return "应用"
	}
}

// consoleImageSubject:appID 非空(应用调用)时强制应用绑定的分组
// (跳过调用者分组可见性)并携带应用快照供使用日志记录。
func consoleImageSubject(subject *coreidentity.Subject, groupID, appID, appName, appOwnerType, appOwnerTenantID, appOwnerUserID string) *coreidentity.Subject {
	out := consoleSubjectForSession(subject, groupID)
	if out != nil && strings.TrimSpace(appID) != "" {
		out.ForcedGroupID = strings.TrimSpace(groupID)
		out.AppID = appID
		out.AppName = appName
		out.AppOwnerType = appOwnerType
		out.AppOwnerTenantID = appOwnerTenantID
		out.AppOwnerUserID = appOwnerUserID
	}
	return out
}

func consoleImageSubjectFromAgent(subject *coreidentity.Subject, groupID string, agent *consoleChatAgentRuntime) *coreidentity.Subject {
	if agent != nil {
		return consoleImageSubject(subject, groupID, agent.ID, agent.Name, agent.OwnerType, agent.OwnerTenantID, agent.OwnerUserID)
	}
	return consoleImageSubject(subject, groupID, "", "", "", "", "")
}

func agentAppID(agent *consoleChatAgentRuntime) string {
	if agent == nil {
		return ""
	}
	return agent.ID
}

func agentAppName(agent *consoleChatAgentRuntime) string {
	if agent == nil {
		return ""
	}
	return agent.Name
}

func agentAppOwnerType(agent *consoleChatAgentRuntime) string {
	if agent == nil {
		return ""
	}
	return agent.OwnerType
}

func agentAppOwnerTenantID(agent *consoleChatAgentRuntime) string {
	if agent == nil {
		return ""
	}
	return agent.OwnerTenantID
}

func agentAppOwnerUserID(agent *consoleChatAgentRuntime) string {
	if agent == nil {
		return ""
	}
	return agent.OwnerUserID
}

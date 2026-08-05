package application

import "strings"

type AppTemplateID string
type AppCapability string

const (
	AppTemplateStandardChat       AppTemplateID = "standard_chat"
	AppTemplateKeywordSelector    AppTemplateID = "keyword_selector"
	AppTemplateDynamicComposition AppTemplateID = "dynamic_prompt_composition"
	AppTemplateTextToImage        AppTemplateID = "text_to_image"
	AppTemplateImageToImage       AppTemplateID = "image_to_image"

	AppCapabilityChat            AppCapability = "chat"
	AppCapabilityImageGeneration AppCapability = "image_generation"
	AppCapabilityImageEdit       AppCapability = "image_edit"
)

type AppTemplate struct {
	ID                  AppTemplateID
	Name                string
	Description         string
	DefaultCapability   AppCapability
	AllowedCapabilities []AppCapability
	PromptStrategy      PromptStrategy
	MinPromptBindings   int
	MaxPromptBindings   int
}

var appTemplates = []AppTemplate{
	{
		ID: AppTemplateStandardChat, Name: "标准对话助手",
		Description:       "直接使用调用方输入进行对话，不绑定提示词。",
		DefaultCapability: AppCapabilityChat, AllowedCapabilities: []AppCapability{AppCapabilityChat},
		PromptStrategy: PromptStrategyNone,
	},
	{
		ID: AppTemplateKeywordSelector, Name: "关键词选择应用",
		Description:         "绑定一条提示词，由调用方通过 variables 填充其中的占位符。",
		DefaultCapability:   AppCapabilityChat,
		AllowedCapabilities: []AppCapability{AppCapabilityChat, AppCapabilityImageGeneration, AppCapabilityImageEdit},
		PromptStrategy:      PromptStrategyCallerVariables, MinPromptBindings: 1, MaxPromptBindings: 1,
	},
	{
		ID: AppTemplateDynamicComposition, Name: "动态提示词组合",
		Description:         "按调用方 input 中的提示词名称精确匹配并组合已绑定提示词。",
		DefaultCapability:   AppCapabilityChat,
		AllowedCapabilities: []AppCapability{AppCapabilityChat, AppCapabilityImageGeneration, AppCapabilityImageEdit},
		PromptStrategy:      PromptStrategyBoundExact, MinPromptBindings: 1, MaxPromptBindings: MaxPromptPlaceholders,
	},
	{
		ID: AppTemplateTextToImage, Name: "文生图应用",
		Description:       "直接使用调用方 input 生成图片，不绑定提示词。",
		DefaultCapability: AppCapabilityImageGeneration, AllowedCapabilities: []AppCapability{AppCapabilityImageGeneration},
		PromptStrategy: PromptStrategyNone,
	},
	{
		ID: AppTemplateImageToImage, Name: "图生图应用",
		Description:       "根据调用方 input 和输入图片生成新图片，不绑定提示词。",
		DefaultCapability: AppCapabilityImageEdit, AllowedCapabilities: []AppCapability{AppCapabilityImageEdit},
		PromptStrategy: PromptStrategyNone,
	},
}

func ListAppTemplates() []AppTemplate {
	out := make([]AppTemplate, len(appTemplates))
	for index, template := range appTemplates {
		out[index] = template
		out[index].AllowedCapabilities = append([]AppCapability(nil), template.AllowedCapabilities...)
	}
	return out
}

func FindAppTemplate(id string) (AppTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, template := range appTemplates {
		if string(template.ID) == id {
			return template, true
		}
	}
	return AppTemplate{}, false
}

func (t AppTemplate) AllowsCapability(capability AppCapability) bool {
	for _, allowed := range t.AllowedCapabilities {
		if allowed == capability {
			return true
		}
	}
	return false
}

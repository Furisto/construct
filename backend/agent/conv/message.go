package conv

import (
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
)

func ConvertMemoryMessageToModel(m *memory.Message) (model.Message, error) {
	source := model.MessageSourceUser
	if m.Role == types.MessageRoleAssistant {
		source = model.MessageSourceModel
	}
	
	contentBlocks := make([]model.ContentBlock, 0)
	if m.Content != nil {
		for _, block := range m.Content.Blocks {
			switch block.Type {
			case types.MessageContentBlockTypeText:
				contentBlocks = append(contentBlocks, &model.TextContentBlock{
					Text: block.Text,
				})
			}
		}
	}
	
	return model.Message{
		Source:  source,
		Content: contentBlocks,
	}, nil
}

func ConvertModelMessageToMemory(m *model.Message) (*types.MessageContent, error) {
	blocks := make([]types.MessageContentBlock, 0, len(m.Content))
	
	for _, block := range m.Content {
		switch block.Type() {
		case model.ContentBlockTypeText:
			textBlock := block.(*model.TextContentBlock)
			blocks = append(blocks, types.MessageContentBlock{
				Type: types.MessageContentBlockTypeText,
				Text: textBlock.Text,
			})
		}
	}
	
	return &types.MessageContent{
		Blocks: blocks,
	}, nil
}

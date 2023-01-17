package channelbot

import (
	"fmt"
	tele "github.com/dontsellfish/telebot_local"
	"log"
	"strings"
)

const OfficialTelegramChannelBotId = 777000

func mediaGroupToId(msg *tele.Message) string {
	if msg.AlbumID != "" {
		return msg.AlbumID
	} else {
		return fmt.Sprintf("%d_%d", msg.Chat.ID, msg.ID)
	}
}

func sendAll(bot *tele.Bot, users []int64, text ...string) {
	for _, user := range users {
		_, _ = bot.Send(&tele.Chat{ID: user}, strings.Join(text, "\n"))
	}
}

func isPersonalMessage(message *tele.Message) bool {
	return message.Chat.ID == message.Sender.ID
}

func personalMessagesOnly(next tele.HandlerFunc) tele.HandlerFunc {
	return func(ctx tele.Context) error {
		if ctx.Chat().ID == ctx.Sender().ID {
			return next(ctx)
		}
		return nil
	}
}

func escapeTgMarkdownV2SpecialSymbols(text string) string {
	// escape chars: '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!'
	replacer := strings.NewReplacer("_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`", ">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-", "=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}", ".", "\\.", "!", "\\!")
	return replacer.Replace(text)
}

func tgMessageToMarkdown(text string, entities tele.Entities) string {
	markdownString := make([]string, len(text))
	for i, char := range []rune(text) {
		markdownString[i] = escapeTgMarkdownV2SpecialSymbols(string(char))
	}

	for _, entity := range entities {
		switch entity.Type {
		case tele.EntityItalic:
			markdownString[entity.Offset] = "_" + markdownString[entity.Offset]
			markdownString[entity.Offset+entity.Length-1] += "_"
		case tele.EntityBold:
			markdownString[entity.Offset] = "*" + markdownString[entity.Offset]
			markdownString[entity.Offset+entity.Length-1] += "*"
		case tele.EntityStrikethrough:
			markdownString[entity.Offset] = "~" + markdownString[entity.Offset]
			markdownString[entity.Offset+entity.Length-1] += "~"
		case tele.EntitySpoiler:
			markdownString[entity.Offset] = "||" + markdownString[entity.Offset]
			markdownString[entity.Offset+entity.Length-1] += "||"
		case tele.EntityCode:
			markdownString[entity.Offset] = "`" + markdownString[entity.Offset]
			markdownString[entity.Offset+entity.Length-1] += "`"
		case tele.EntityCodeBlock:
			markdownString[entity.Offset] = "```" + markdownString[entity.Offset]
			markdownString[entity.Offset+entity.Length-1] += "```"
		case tele.EntityTextLink:
			markdownString[entity.Offset] = "[" + markdownString[entity.Offset]
			markdownString[entity.Offset+entity.Length-1] += fmt.Sprintf("](%s)", escapeTgMarkdownV2SpecialSymbols(entity.URL))
		case tele.EntityMention:
		case tele.EntityURL:
		default:
			log.Printf("entity of type %s is not supported", entity.Type)
		}
	}

	return strings.Join(markdownString, "")
}

package channelbot

import (
	"encoding/json"
	"errors"
	tele "github.com/dontsellfish/telebot_local"
	"os"
	"path"
	"strings"
	"time"
)

/*
struct PostInfo:
	time - TimeString
	post_text.md - string
	comment_text.md - string
	post_sources - enum (PostSourcesAuto, PostSourcesTrue, PostSourcesFalse)
	is_protected - bool
	file_type_1 tg_file_id_1 file_type_2 tg_file_id_2... - List[(TelegramFileType, string)]
	msg_id_in_comments_chat_channel_posted - int
	original_msg_ids	- List[int]

TelegramFileType - enum (TelegramFileTypePhoto, TelegramFileTypeVideo, TelegramFileTypeDocPhoto, TelegramFileTypeDocVideo)
*/

const (
	PostSourcesAuto = iota
	PostSourcesTrue
	PostSourcesFalse
)

const (
	TelegramFileTypePhoto = iota
	TelegramFileTypeVideo
	TelegramFileTypeDocPhoto
	TelegramFileTypeDocVideo
)

const (
	ChangePostTime = iota
	ChangePostText
	ChangePostComment
	ChangePostPostSources
	ChangePostIsProtected
	ChangePostMsgIdInCommentsChat
)

const TimeIsNotSpecified = "NA"

type TgFileInfo struct {
	Type int
	Id   string
}

type MessageLink struct {
	ChatId    int64 `json:"chat-id"`
	MessageId int   `json:"message-id"`
}

type Post struct {
	Id             string        `json:"id"`
	ScheduledTime  string        `json:"time"`
	MessagesInChat []MessageLink `json:"admin-messages"`

	AsSources bool         `json:"sources,omitempty"`
	Text      string       `json:"text,omitempty"`
	Protected bool         `json:"protected,omitempty"`
	Reply     MessageLink  `json:"reply,omitempty"`
	Files     []TgFileInfo `json:"files"`

	Comment *Post `json:"comment,omitempty"`
}

func (post *Post) MarshalBinary() ([]byte, error) {
	return json.Marshal(post)
}

func (post *Post) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, &post)
}

func (post *Post) GetPoster() int64 {
	if len(post.MessagesInChat) == 0 {
		return 0
	}
	return post.MessagesInChat[0].ChatId
}

func (post *Post) ToAlbum(bot *ChannelBot) (tele.Album, error) {
	album := tele.Album{}
	for i, postFile := range post.Files {
		caption := ""
		if i+1 == len(post.Files) {
			caption = post.Text
		}

		file := tele.File{FileID: postFile.Id}
		switch postFile.Type {
		case TelegramFileTypePhoto:
			album = append(album, &tele.Photo{
				File:    file,
				Caption: caption,
			})
		case TelegramFileTypeVideo:
			album = append(album, &tele.Video{
				File:    file,
				Caption: caption,
			})
		case TelegramFileTypeDocPhoto:
			localFileName := path.Join(bot.Config.TemporaryFilesDirectory, postFile.Id)
			err := bot.Telegram.Download(&file, localFileName)
			if err != nil {
				return nil, err
			}
			go func() {
				time.Sleep(time.Minute * 5)
				_ = os.Remove(localFileName)
			}()

			//imageForTelegram, err := bot.Converter.ImageTelegram(localFileName)
			//if err != nil {
			//	return nil, err
			//}
			go func() {
				time.Sleep(time.Minute * 5)
				//_ = os.Remove(imageForTelegram)
				//if imageForTelegram != localFileName {
				_ = os.Remove(localFileName)
				//}
			}()

			album = append(album, &tele.Photo{
				File:    tele.FromDisk(localFileName),
				Caption: caption,
			})
		case TelegramFileTypeDocVideo:
			localFileName := path.Join(bot.Config.TemporaryFilesDirectory, postFile.Id)
			err := bot.Telegram.Download(&file, localFileName)
			if err != nil {
				return nil, err
			}
			go func() {
				time.Sleep(time.Minute * 5)
				_ = os.Remove(localFileName)
			}()
			album = append(album, &tele.Video{
				File:    tele.FromDisk(localFileName),
				Caption: caption,
			})
		}
	}

	return album, nil
}

func (post *Post) ToDocumentsAlbum() (tele.Album, error) {
	album := tele.Album{}
	for i, postFile := range post.Files {
		caption := ""
		if i+1 == len(post.Files) {
			caption = post.Text
		}

		file := tele.File{FileID: postFile.Id}
		switch postFile.Type {
		case TelegramFileTypeDocPhoto:
			album = append(album, &tele.Document{
				File:    file,
				Caption: caption,
			})
		case TelegramFileTypeDocVideo:
			album = append(album, &tele.Document{
				File:    file,
				Caption: caption,
			})
		}
	}
	return album, nil
}

func (post *Post) SendReply(bot *ChannelBot, message MessageLink) ([]tele.Message, error) {
	post.Reply = message
	return post.Send(bot, &tele.Chat{ID: post.Reply.ChatId})
}

func (post *Post) ToSendOptions() *tele.SendOptions {
	return &tele.SendOptions{
		ReplyTo:           &tele.Message{ID: post.Reply.MessageId, Chat: &tele.Chat{ID: post.Reply.ChatId}},
		ParseMode:         DefaultParseMode,
		Protected:         post.Protected,
		AllowWithoutReply: true,
	}
}

func (post *Post) IsDocuments() bool {
	for _, file := range post.Files {
		if file.Type != TelegramFileTypeDocPhoto && file.Type != TelegramFileTypeDocVideo {
			return false
		}
	}
	return true
}

func (post *Post) Clone() *Post {
	clone := deepCopyViaJsonSorryJesusChrist(post)
	clone.Id = post.Id + "_cloned"
	return clone
}

func (post *Post) Send(bot *ChannelBot, to tele.Recipient) ([]tele.Message, error) {
	if len(post.Files) == 0 {
		message, err := bot.Telegram.Send(to, post.Text, post.ToSendOptions())
		if err != nil {
			return nil, err
		} else {
			return []tele.Message{*message}, nil
		}
	} else if post.AsSources {
		album, err := post.ToDocumentsAlbum()
		if err != nil {
			return nil, err
		} else {
			return bot.Telegram.SendAlbum(to, album, post.ToSendOptions())
		}
	} else {
		album, err := post.ToAlbum(bot)
		if err != nil {
			return nil, err
		} else {
			return bot.Telegram.SendAlbum(to, album, post.ToSendOptions())
		}
	}
}

func PostFromMessages(messages []*tele.Message) (*Post, error) {
	post := Post{
		Id:             mediaGroupToId(messages[0]),
		ScheduledTime:  TimeIsNotSpecified,
		MessagesInChat: make([]MessageLink, len(messages)),
		AsSources:      false,
		Text:           "",
		Protected:      false,
		Reply:          MessageLink{},
		Files:          make([]TgFileInfo, len(messages)),
		Comment:        nil,
	}

	for i, msg := range messages {
		post.MessagesInChat[i] = MessageLink{MessageId: msg.ID, ChatId: msg.Chat.ID}
		switch {
		case msg.Photo != nil:
			post.Files[i] = TgFileInfo{TelegramFileTypePhoto, msg.Photo.FileID}
		case msg.Video != nil:
			post.Files[i] = TgFileInfo{TelegramFileTypeVideo, msg.Video.FileID}
		case msg.Document != nil && strings.HasPrefix(strings.ToLower(msg.Document.MIME), "image"):
			post.Files[i] = TgFileInfo{TelegramFileTypeDocPhoto, msg.Document.FileID}
		case msg.Document != nil && strings.HasPrefix(strings.ToLower(msg.Document.MIME), "video"):
			post.Files[i] = TgFileInfo{TelegramFileTypeDocVideo, msg.Document.FileID}
		default:
			return nil, errors.New("message with no supported media is provided")
		}
	}

	return &post, nil
}

func PostFromText(message *tele.Message) *Post {
	return &Post{
		Id:             mediaGroupToId(message),
		ScheduledTime:  TimeIsNotSpecified,
		MessagesInChat: []MessageLink{{MessageId: message.ID, ChatId: message.Chat.ID}},
		AsSources:      false,
		Text:           message.Text,
		Protected:      false,
		Reply:          MessageLink{},
		Files:          []TgFileInfo{},
		Comment:        nil,
	}
}

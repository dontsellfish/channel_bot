package channelbot

import (
	"encoding/json"
	tele "github.com/dontsellfish/telebot_local"
	"os"
)

const (
	DefaultTemporaryFilesDirectory = "."
	DefaultRedisPrefix             = "channelbot"
	DefaultRedisAddress            = "localhost:6379"
	DefaultParseMode               = tele.ModeMarkdownV2
	DefaultStartMessage            = "Hmm?.."
	DefaultDefaultPostText         = ""
)

type Config struct {
	Token            string   `json:"token"`
	AdminList        []int64  `json:"admin-list"`
	DefaultPostTimes []string `json:"default-post-times"`
	ChannelId        int64    `json:"channel-id"`
	CommentsId       int64    `json:"comments-id"`
	StartMessage     string   `json:"start-message"`

	TemporaryFilesDirectory string `json:"temporary-files-directory,omitempty"`
	RedisPrefix             string `json:"redis-prefix,omitempty"`
	RedisAddress            string `json:"redis-address,omitempty"`
	RedisDatabaseNumber     int    `json:"redis-database-number,omitempty"`

	DefaultPostText       string `json:"default-post-text,omitempty"`
	DisableWebPagePreview bool   `json:"disable-web-page-preview,omitempty"`
	DisableNotification   bool   `json:"disable-notification,omitempty"`
	Verbose               bool   `json:"verbose,omitempty"`
	Local                 bool   `json:"local,omitempty"`
	Sync                  bool   `json:"sync,omitempty"`

	ConfigPath string `json:"config-path,omitempty"`
}

func (cfg Config) FillDefaults() Config {
	if cfg.TemporaryFilesDirectory == "" {
		cfg.TemporaryFilesDirectory = DefaultTemporaryFilesDirectory
	}
	if cfg.RedisPrefix == "" {
		cfg.RedisPrefix = DefaultRedisPrefix
	}
	if cfg.RedisAddress == "" {
		cfg.RedisAddress = DefaultRedisAddress
	}
	if cfg.DefaultPostText == "" {
		cfg.DefaultPostText = DefaultDefaultPostText
	}
	if cfg.StartMessage == "" {
		cfg.StartMessage = DefaultStartMessage
	}
	return cfg
}

func LoadConfig(filename string) (cfg Config, err error) {
	cfg.ConfigPath = filename
	buff, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	err = json.Unmarshal(buff, &cfg)
	if err != nil {
		return
	}
	return cfg, nil
}

func (cfg Config) DumpTo(filename string) error {
	buffer, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, buffer, os.ModePerm)
}

func (cfg Config) Dump() error {
	return cfg.DumpTo(cfg.ConfigPath)
}

var ChannelBotCommands = []tele.Command{
	{
		Text:        "/info",
		Description: "info about the database",
	}, {
		Text:        "/preview",
		Description: "[all] get the post preview in this chat",
	}, {
		Text:        "/post",
		Description: "post the post immediately",
	}, {
		Text:        "/random",
		Description: "make a random post immediately",
	}, {
		Text:        "/remove",
		Description: "delete the post from the database",
	}, {
		Text:        "/schedule",
		Description: "[HH:MM...] change schedule",
	}, {
		Text:        "/notext",
		Description: "clear text of the post",
	}, {
		Text:        "/nocomment",
		Description: "remove the comment of the post",
	}, {
		Text:        "/nocommenttext",
		Description: "clear text of a post's comment",
	}, {
		Text:        "/source",
		Description: "post sources in the comments of the post",
	}, {
		Text:        "/docs",
		Description: "convert docs to images in comments (or vice-versa)",
	}, {
		Text:        "/protected",
		Description: "make post protected/unprotected",
	}, {
		Text:        "/clear",
		Description: "[all] remove all post from DB",
	}, {
		Text:        "/shutdown",
		Description: "manually shutdown the bot, usage: /shutdown please",
	},
}

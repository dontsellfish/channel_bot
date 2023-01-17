package main

import (
	"channel_bot/channelbot"
	"log"
)

func main() {
	bot, err := channelbot.FromFile("octorat.json")
	if err != nil {
		log.Fatalln(err)
	}
	bot.Start()
}

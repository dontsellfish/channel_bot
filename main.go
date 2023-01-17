package main

import (
	"channel_bot/channelbot"
	"flag"
	"log"
)

const DefaultConfigPath = "./cfg.json"

func main() {
	config := flag.String("c", DefaultConfigPath, "path to a config file")
	flag.Parse()

	bot, err := channelbot.FromFile(*config)
	if err != nil {
		log.Fatalln(err)
	}
	bot.Start()
}

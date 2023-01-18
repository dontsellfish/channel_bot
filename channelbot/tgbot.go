package channelbot

import (
	"encoding/json"
	"errors"
	"fmt"
	tele "github.com/dontsellfish/telebot_local"
	"github.com/go-redis/redis/v8"
	"os"
	"regexp"
	"strings"
	"time"
)

type ChannelBot struct {
	Telegram  *tele.Bot
	Config    Config
	Database  *Database
	Converter *Converter
}

func FromFile(filename string) (*ChannelBot, error) {
	cfg, err := LoadConfig(filename)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

func New(config Config) (bot *ChannelBot, err error) {
	bot = &ChannelBot{Telegram: nil, Config: config.FillDefaults(), Converter: NewConverter()}
	bot.Telegram, err = tele.NewBot(tele.Settings{
		Token:       bot.Config.Token,
		URL:         bot.Config.Url,
		Poller:      &tele.LongPoller{Timeout: time.Minute},
		Synchronous: config.Sync,
		Verbose:     config.Verbose,
		Local:       config.Local,
		OnError: func(err error, ctx tele.Context) {
			if IsErrRedisNotFound(err) {
				err = errors.New("not found in the database")
			}
			msg := ctx.Message()
			if msg == nil {
				msg = &tele.Message{Chat: &tele.Chat{}}
			}
			report := fmt.Sprintf("Error: %v\nChat: %s %s %s (%d)\nMessageId: %d",
				err,
				msg.Chat.FirstName,
				msg.Chat.LastName,
				msg.Chat.Title,
				msg.Chat.ID,
				msg.ID,
			)
			sendAll(ctx.Bot(), config.AdminList, report)
		},
	})
	if err != nil {
		return nil, err
	}

	err = createDirectoryIfNotFound(bot.Config.TemporaryFilesDirectory)
	if err != nil {
		return nil, err
	}

	_, err = bot.Telegram.ChatMemberOf(&tele.Chat{ID: bot.Config.ChannelId}, bot.Telegram.Me)
	if err != nil {
		bot.alertAdmins("bot is not member of the channel")
	}
	_, err = bot.Telegram.ChatMemberOf(&tele.Chat{ID: bot.Config.CommentsId}, bot.Telegram.Me)
	if err != nil {
		bot.alertAdmins("bot is not member of the comments chat")
	}

	bot.Database = NewDatabase(fmt.Sprintf("%s:%d", bot.Config.RedisPrefix, bot.Telegram.Me.ID), &redis.Options{
		Addr: bot.Config.RedisAddress,
		DB:   bot.Config.RedisDatabaseNumber,
	})

	return bot, nil
}

func (bot *ChannelBot) Start() {
	HandleAlbum(bot.Telegram, func(msgs []*tele.Message) error {
		switch {
		case isPersonalMessage(msgs[0]) && containsInt(bot.Config.AdminList, msgs[0].Chat.ID):
			post, err := PostFromMessages(msgs)
			if err != nil {
				return err
			}
			if msgs[0].ReplyTo != nil {
				orig, err := bot.Database.GetPostByMessageLink(MessageLink{MessageId: msgs[0].ReplyTo.ID, ChatId: msgs[0].Chat.ID})
				if err == nil {
					post.AsSources = post.IsDocuments()
					msg, _ := bot.Telegram.Reply(msgs[0], "+ (comment)")
					if msg != nil {
						bot.MakeExpiring(time.Second*15, *msg)
					}
					if orig.Comment != nil {
						post.Text = orig.Comment.Text
					}
					return bot.Database.AddComment(orig.Id, post)
				}
			}

			msg, _ := bot.Telegram.Reply(msgs[0], "+ (post)")
			if msg != nil {
				bot.MakeExpiring(time.Second*15, *msg)
			}
			post.Text = bot.Config.DefaultPostText
			return bot.Database.SetPost(post.Id, post)

		case msgs[0].Chat.ID == bot.Config.CommentsId && msgs[0].IsForwarded() && msgs[0].Sender.ID == OfficialTelegramChannelBotId:
			post, err := bot.Database.GetRecentlyPosted(msgs[0].OriginalMessageID)
			if err != nil {
				if IsErrRedisNotFound(err) {
					return nil
				} else {
					return err
				}
			}
			_, err = post.Comment.SendReply(bot, MessageLink{msgs[0].Chat.ID, msgs[0].ID})
			if err != nil {
				bot.Telegram.OnError(err, bot.Telegram.NewContext(tele.Update{Message: msgs[0]}))
			}

			err = bot.Database.RemPost(post)
			if err != nil {
				bot.Telegram.OnError(err, bot.Telegram.NewContext(tele.Update{Message: msgs[0]}))
			}

			return err

		default:
			return nil
		}
	})

	admin := bot.Telegram.Group()
	admin.Use(bot.adminOnly)
	admin.Use(personalMessagesOnly)
	admin.Handle("/post", func(ctx tele.Context) error {
		post, err := bot.getReferredPost(ctx)
		if err != nil {
			return err
		}
		msg, err := bot.Telegram.Reply(ctx.Message(), "+")
		if err == nil {
			bot.MakeExpiring(time.Second*15, *msg)
		}
		return bot.makeChannelPostWithComments(post)
	})
	admin.Handle("/random", func(ctx tele.Context) error {
		post, err := bot.Database.GetRandomPostByTime(TimeIsNotSpecified)
		if err != nil {
			return err
		}
		_, _ = bot.Telegram.Reply(&tele.Message{ID: post.MessagesInChat[0].MessageId, Chat: &tele.Chat{ID: post.MessagesInChat[0].ChatId}}, "+")
		return bot.makeChannelPostWithComments(post)
	})
	admin.Handle("/preview", func(ctx tele.Context) error {
		posts := []*Post{}
		var err error
		if len(ctx.Args()) != 0 && strings.ToLower(ctx.Args()[0]) == "all" {
			posts, err = bot.Database.GetAllPosts()
			if err != nil {
				bot.Telegram.OnError(err, ctx)
			}
		} else {
			post, err := bot.getReferredPost(ctx)
			if err != nil {
				return err
			}
			posts = append(posts, post)
		}

		for _, post := range posts {
			messages, err := post.SendReply(bot, post.MessagesInChat[0])
			if err != nil {
				bot.Telegram.OnError(err, ctx)
			} else {
				bot.MakeExpiring(time.Minute*5, messages...)
			}
			if post.Comment != nil && len(messages) > 0 && messages[0].Chat != nil {
				messages, err = post.Comment.SendReply(bot, MessageLink{messages[0].Chat.ID, messages[0].ID})
				if err != nil {
					bot.Telegram.OnError(err, ctx)
				} else {
					bot.MakeExpiring(time.Minute*5, messages...)
				}
			}
			time.Sleep(time.Millisecond * 100)
		}

		return nil
	})
	admin.Handle("/info", func(ctx tele.Context) error {
		report, err := bot.Database.Report()
		if err != nil {
			return err
		}

		return ctx.Send(strings.Join([]string{
			report,
			fmt.Sprintf("Schedule: %s\n~%.2f days covered with posts.",
				strings.Join(bot.Config.DefaultPostTimes, " "),
				float64(bot.Database.Size())/float64(len(bot.Config.DefaultPostTimes))),
		}, "\n\n"))
	})
	admin.Handle("/shutdown", func(ctx tele.Context) error {
		if len(ctx.Args()) > 0 && strings.ToLower(ctx.Args()[0]) == "please" {
			_ = ctx.Reply("shutting down...")
			os.Exit(0)
		} else {
			return ctx.Reply("say 'please', be gentle")
		}
		return nil
	})
	admin.Handle("/clear", func(ctx tele.Context) error {
		if len(ctx.Args()) > 0 && strings.ToLower(ctx.Args()[0]) == "all" {
			posts, err := bot.Database.GetAllPosts()
			if err != nil {
				return err
			}
			for _, post := range posts {
				_ = bot.Database.RemPost(post)
			}
			return ctx.Reply("Cleared.")
		} else {
			return ctx.Reply("say 'all', to be sure")
		}
	})
	timeRegex := regexp.MustCompile("^([0-1][0-9]|2[0-3]):[0-5][0-9]$")
	admin.Handle("/schedule", func(ctx tele.Context) error {
		for _, t := range ctx.Args() {
			if !timeRegex.MatchString(t) {
				return ctx.Reply(fmt.Sprintf("Time %s is invalid", t))
			}
		}

		_, _ = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Old: '%s'\nNew: '%s'",
			strings.Join(bot.Config.DefaultPostTimes, " "), strings.Join(ctx.Args(), " ")))

		bot.Config.DefaultPostTimes = ctx.Args()
		err := bot.Config.Dump()
		if err != nil {
			return err
		}

		return nil
	})

	admin.Handle(tele.OnText, func(ctx tele.Context) error {
		post, err := bot.getReferredPost(ctx)
		if err != nil {
			if IsErrRedisNotFound(err) {
				return ctx.Reply("hm?")
			} else {
				return err
			}
		}

		var message *tele.Message
		if timeRegex.Match([]byte(ctx.Text())) {
			message, err = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Time '%s' --> '%s'", post.ScheduledTime, ctx.Text()))
			post.ScheduledTime = ctx.Text()
		} else if ctx.Text() == "/source" {
			if !post.IsDocuments() {
				message, err = bot.Telegram.Reply(ctx.Message(), "Nothing could be changed.")
			} else {
				sources := post.Clone()
				sources.AsSources = true
				sources.Text = ""
				message, _ = bot.Telegram.Reply(ctx.Message(), "Sources shall be posted.")
				err = bot.Database.AddComment(post.Id, sources)
			}
		} else if ctx.Text() == "/docs" {
			if post.Comment == nil || !post.Comment.IsDocuments() {
				message, err = bot.Telegram.Reply(ctx.Message(), "Nothing could be changed.")
			} else {
				message, _ = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Converting sources to pictures (%t) --> (%t)", !post.Comment.AsSources, post.Comment.AsSources))
				post.Comment.AsSources = !post.Comment.AsSources
				err = bot.Database.EditPost(post)
			}
		} else if ctx.Text() == "/remove" {
			err = bot.Database.RemPost(post)
			if err == nil {
				message, err = bot.Telegram.Reply(ctx.Message(), "Removed")
			}
		} else if ctx.Text() == "/notext" {
			message, _ = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Post text '%s' --> ''", post.Text))
			post.Text = ""
			err = bot.Database.EditPost(post)
		} else if ctx.Text() == "/nocomment" {
			message, _ = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Comment is removed."))
			post.Comment = nil
			err = bot.Database.EditPost(post)
		} else if ctx.Text() == "/nocommenttext" {
			if post.Comment == nil {
				message, err = bot.Telegram.Reply(ctx.Message(), "Nothing could be changed.")
			} else {
				message, _ = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Comment text '%s' --> ''", post.Comment.Text))
				post.Comment.Text = ""
				err = bot.Database.EditPost(post)
			}
		} else if ctx.Text() == "/protected" {
			message, _ = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Post protection (%t) --> (%t)", post.Protected, !post.Protected))
			post.Protected = !post.Protected
			if post.Comment != nil {
				post.Comment.Protected = post.Protected
			}
			err = bot.Database.EditPost(post)
		} else if ctx.Text() == "/debug" {
			var buffer []byte
			buffer, err = json.MarshalIndent(post, "", "    ")
			if err == nil {
				message, err = bot.Telegram.Reply(ctx.Message(), string(buffer))
			} else {
				message, err = bot.Telegram.Reply(ctx.Message(), err.Error())
			}
		} else if strings.HasSuffix(ctx.Text(), ".p") {
			text := tgMessageToMarkdown(ctx.Text()[:len(ctx.Text())-len(".p")], ctx.Message().Entities)
			message, err = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Post text '%s' --> '%s'", post.Text, text))
			post.Text = text
		} else {
			text := tgMessageToMarkdown(ctx.Text(), ctx.Message().Entities)
			message, err = bot.Telegram.Reply(ctx.Message(), fmt.Sprintf("Comment text '%s' --> '%s'", post.Text, text))
			if post.Comment != nil {
				post.Comment.Text = text
			} else {
				post.Comment = PostFromText(ctx.Message())
			}
			err = bot.Database.EditPost(post)
		}
		if err != nil && message != nil {
			bot.MakeExpiring(time.Second*15, *message)
		}

		return bot.Database.EditPost(post)
	})
	bot.Telegram.Handle("/start", func(ctx tele.Context) error {
		return ctx.Send(bot.Config.StartMessage)
	})

	go bot.startTimeBasedPostingRoutine()
	_ = bot.Telegram.SetCommands(ChannelBotCommands)
	go bot.Telegram.Start()

	select {}
}

func (bot *ChannelBot) getReferredPost(ctx tele.Context) (*Post, error) {
	if ctx.Message().ReplyTo == nil {
		return nil, errors.New("no message is provided")
	}
	return bot.Database.GetPostByMessageLink(MessageLink{MessageId: ctx.Message().ReplyTo.ID, ChatId: ctx.Message().ReplyTo.Chat.ID})
}

func (bot *ChannelBot) alertAdmins(text ...string) {
	sendAll(bot.Telegram, bot.Config.AdminList, text...)
}

func (bot *ChannelBot) startTimeBasedPostingRoutine() {
	time.Sleep(time.Duration(60+5-time.Now().Second()) * time.Second)
	err := bot.ifItIsTimePostRandom(time.Now().Format("15:04"), 4)
	if err != nil {
		bot.alertAdmins("WHILE TRYING TO POST", err.Error())
	}
	for tick := range time.Tick(time.Minute) {
		err = bot.ifItIsTimePostRandom(tick.Format("15:04"), 4)
		if err != nil {
			bot.alertAdmins("WHILE TRYING TO POST", err.Error())
		}
	}
}

func (bot *ChannelBot) ifItIsTimePostRandom(t string, retries int, errs ...string) error {
	if retries >= 0 {
		post, err := bot.Database.GetRandomPostByTime(t)
		pointBrokenPost := func(err error) {
			_, postErr := bot.Telegram.Reply(&tele.Message{ID: post.MessagesInChat[0].MessageId, Chat: &tele.Chat{ID: post.MessagesInChat[0].ChatId}},
				fmt.Sprintf("an error while trying to post\n%s", err.Error()))
			errs = append(errs, err.Error())
			if postErr != nil {
				errs = append(errs, postErr.Error())
			}
		}
		if err != nil {
			if !IsErrRedisNotFound(err) {
				return errors.New(err.Error() + " while getting random post for time " + t)
			} else if t != TimeIsNotSpecified && contains(bot.Config.DefaultPostTimes, t) {
				return bot.ifItIsTimePostRandom(TimeIsNotSpecified, retries, errs...)
			}
		} else {
			err = bot.makeChannelPostWithComments(post)
			if err != nil {
				pointBrokenPost(err)
				return errors.New("an error while trying to post " + post.Id + err.Error())
			}
		}
	}

	if len(errs) == 0 {
		return nil
	} else {
		return errors.New(strings.Join(errs, "\n"))
	}
}

func (bot *ChannelBot) MakeExpiring(duration time.Duration, messages ...tele.Message) {
	go func() {
		time.Sleep(duration)
		for _, msg := range messages {
			err := bot.Telegram.Delete(&msg)
			if err != nil {
				bot.Telegram.OnError(err, bot.Telegram.NewContext(tele.Update{Message: &msg}))
			}
		}
	}()
}

func (bot *ChannelBot) makeChannelPostWithComments(post *Post) error {
	messages, err := post.Send(bot, &tele.Chat{ID: bot.Config.ChannelId})
	if err != nil {
		return err
	}

	if post.Comment == nil {
		return bot.Database.RemPost(post)
	}

	return bot.Database.AddRecentlyPosted(post.Id, messages[0].ID)
}

func (bot *ChannelBot) replyExpiring(to *tele.Message, text string) {
	message, err := bot.Telegram.Reply(to, text, &tele.SendOptions{AllowWithoutReply: true})
	if err != nil {
		bot.Telegram.OnError(err, bot.Telegram.NewContext(tele.Update{Message: to}))
		return
	}
	go func() {
		time.Sleep(time.Second * 20)
		err = bot.Telegram.Delete(message)
		if err != nil {
			bot.Telegram.OnError(err, bot.Telegram.NewContext(tele.Update{Message: to}))
			return
		}
	}()
}

func (bot *ChannelBot) adminOnly(next tele.HandlerFunc) tele.HandlerFunc {
	return func(ctx tele.Context) error {
		if containsInt(bot.Config.AdminList, ctx.Sender().ID) {
			return next(ctx)
		}
		return nil
	}
}

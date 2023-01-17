package channelbot

import (
	"errors"
	"fmt"
	tele "github.com/dontsellfish/telebot_local"
	"sort"
	"sync"
	"time"
)

const DefaultTimeoutForHandlingAlbum = time.Second

type AlbumHandler struct {
	Group   *tele.Group
	Handler func(messages []*tele.Message) error
	Timeout time.Duration

	albums        map[string][]*tele.Message
	registerMutex sync.Mutex
}

func HandleAlbum(botOrGroup interface{}, handler func(messages []*tele.Message) error, m ...tele.MiddlewareFunc) {
	var group *tele.Group
	switch botOrGroup.(type) {
	case *tele.Bot:
		group = botOrGroup.(*tele.Bot).Group()
	case *tele.Group:
		group = botOrGroup.(*tele.Group)
	default:
		panic("the first argument has to be (*telebot.Bot) or (*telebot.Group)")
	}
	albumHandler := AlbumHandler{
		Group:   group,
		Handler: handler,
		Timeout: DefaultTimeoutForHandlingAlbum,

		albums:        map[string][]*tele.Message{},
		registerMutex: sync.Mutex{},
	}
	group.Handle(tele.OnMedia, func(ctx tele.Context) error { return albumHandler.register(ctx) }, m...)
}

func (handler *AlbumHandler) register(ctx tele.Context) error {
	defer handler.registerMutex.Unlock()
	handler.registerMutex.Lock()

	message := deepCopyViaJsonSorryJesusChrist(ctx.Message())

	id := mediaGroupToId(message)
	if _, contains := handler.albums[id]; !contains {
		handler.albums[id] = []*tele.Message{message}

		go handler.delayHandling(ctx, message, id)
	} else {
		handler.albums[id] = append(handler.albums[id], message)
	}

	return nil
}

func (handler *AlbumHandler) delayHandling(ctx tele.Context, message *tele.Message, id string) {
	defer func() {
		delete(handler.albums, id)
		if r := recover(); r != nil {
			ctx.Bot().OnError(errors.New(fmt.Sprintf("%v", r)), ctx.Bot().NewContext(tele.Update{Message: deepCopyViaJsonSorryJesusChrist(message)}))
		}
	}()
	if message.AlbumID != "" { // no need to delay handling of single medias
		time.Sleep(handler.Timeout)
	}
	messages := handler.albums[mediaGroupToId(message)]
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].ID < messages[j].ID
	})
	err := handler.Handler(messages)
	if err != nil {
		ctx.Bot().OnError(err, ctx.Bot().NewContext(tele.Update{Message: deepCopyViaJsonSorryJesusChrist(message)}))
	}
}

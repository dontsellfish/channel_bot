package channelbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"log"
	"strings"
	"time"
)

var redisContext = context.Background()

type Database struct {
	client *redis.Client
	prefix string
}

func NewDatabase(prefix string, opt *redis.Options) *Database {
	return &Database{
		client: redis.NewClient(opt),
		prefix: prefix,
	}
}

func (db *Database) toKey(args ...string) string {
	entities := []string{db.prefix}
	entities = append(entities, args...)
	return strings.Join(entities, ":")
}

func (db *Database) SetPost(id string, post *Post) error {
	err := db.client.SAdd(redisContext, db.toKey("times"), post.ScheduledTime).Err()
	if err != nil {
		return nil
	}
	err = db.client.SAdd(redisContext, db.toKey("posts"), post.Id).Err()
	if err != nil {
		return nil
	}
	err = db.client.SAdd(redisContext, db.toKey("time", post.ScheduledTime), post.Id).Err()
	if err != nil {
		return nil
	}
	for _, msg := range post.MessagesInChat {
		err = db.client.Set(redisContext,
			db.toKey("admin-chat", fmt.Sprintf("%d", msg.ChatId), "msg-id", fmt.Sprintf("%d", msg.MessageId)),
			post.Id,
			0).Err()
		if err != nil {
			return nil
		}
	}

	return db.client.Set(redisContext, db.toKey("post", id), post, 0).Err()
}

func (db *Database) EditPost(post *Post) error {
	if post == nil {
		return errors.New("post is nil, what")
	}

	logIfError := func(err error) {
		if err != nil {
			log.Printf("warning: while editing post (%s) an error occurred: %s", post.Id, err.Error())
		}
	}

	original, err := db.GetPost(post.Id)
	logIfError(err)

	if original.ScheduledTime != post.ScheduledTime {
		logIfError(db.client.SRem(redisContext, db.toKey("time", original.ScheduledTime), post.Id).Err())
		size, err := db.client.SCard(redisContext, db.toKey("time", original.ScheduledTime)).Result()
		logIfError(err)
		if size == 0 {
			logIfError(db.client.SRem(redisContext, db.toKey("times"), original.ScheduledTime).Err())
		}

		logIfError(db.client.SAdd(redisContext, db.toKey("times"), post.ScheduledTime).Err())
		logIfError(db.client.SAdd(redisContext, db.toKey("time", post.ScheduledTime), post.Id).Err())
	}

	b, _ := json.Marshal(post)
	log.Println("before set", string(b))
	err = db.client.Set(redisContext, db.toKey("post", post.Id), post, 0).Err()
	logIfError(err)
	post, err = db.GetPost(post.Id)
	logIfError(err)
	b, _ = json.Marshal(post)
	log.Println("get after", string(b))

	return nil
}

func (db *Database) RemPost(post *Post) error {
	errs := make([]string, 0)
	err := db.client.SRem(redisContext, db.toKey("time", post.ScheduledTime), post.Id).Err()
	size, err := db.client.SCard(redisContext, db.toKey("time", post.ScheduledTime)).Result()
	if size == 0 {
		err = db.client.SRem(redisContext, db.toKey("times"), post.ScheduledTime).Err()
	}
	err = db.client.SRem(redisContext, db.toKey("posts"), post.Id).Err()
	if err != nil {
		errs = append(errs, err.Error())
	}
	err = db.client.Del(redisContext, db.toKey("post", post.Id)).Err()
	if err != nil {
		errs = append(errs, err.Error())
	}
	for _, msg := range post.MessagesInChat {
		err = db.client.Del(redisContext,
			db.toKey("admin-chat", fmt.Sprintf("%d", msg.ChatId), "msg-id", fmt.Sprintf("%d", msg.ChatId)),
			post.Id).Err()
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) == 0 {
		return nil
	} else {
		return errors.New(strings.Join(errs, "\n"))
	}
}

func (db *Database) GetPost(id string) (*Post, error) {
	buffer, err := db.client.Get(redisContext, db.toKey("post", id)).Bytes()
	if err != nil {
		return nil, err
	}

	var post Post
	err = json.Unmarshal(buffer, &post)
	if err != nil {
		return nil, err
	}

	return &post, nil
}

func (db *Database) AddComment(id string, comment *Post) error {
	post, err := db.GetPost(id)
	if err != nil {
		return err
	}
	post.Comment = comment

	for _, msg := range comment.MessagesInChat {
		_ = db.AddTemporaryMessageLink(msg, id)
	}

	return db.EditPost(post)
}

func (db *Database) AddRecentlyPosted(id string, idInChannel int) error {
	_, err := db.GetPost(id)
	if err != nil {
		return err
	}
	return db.client.Set(redisContext, db.toKey("recent", fmt.Sprintf("%d", idInChannel)), id, time.Minute).Err()
}

func (db *Database) GetRecentlyPosted(idInChannel int) (*Post, error) {
	id, err := db.client.Get(redisContext, db.toKey("recent", fmt.Sprintf("%d", idInChannel))).Bytes()
	if err != nil {
		return nil, err
	}

	return db.GetPost(string(id))
}

func (db *Database) AddTemporaryMessageLink(link MessageLink, id string) error {
	return db.client.Set(redisContext,
		db.toKey("admin-chat", fmt.Sprintf("%d", link.ChatId), "msg-id", fmt.Sprintf("%d", link.MessageId)),
		id,
		time.Hour).Err()
}

func (db *Database) GetPostByMessageLink(link MessageLink) (*Post, error) {
	id, err := db.client.Get(redisContext,
		db.toKey("admin-chat", fmt.Sprintf("%d", link.ChatId), "msg-id", fmt.Sprintf("%d", link.MessageId))).Result()
	if err != nil {
		return nil, err
	}

	return db.GetPost(id)
}

func (db *Database) GetRandomPostByTime(t string) (*Post, error) {
	id, err := db.client.SRandMember(redisContext, db.toKey("time", t)).Bytes()
	if err != nil {
		return nil, err
	}
	buffer, err := db.client.Get(redisContext, db.toKey("post", string(id))).Bytes()
	if err != nil {
		return nil, err
	}

	var post Post
	err = json.Unmarshal(buffer, &post)
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func (db *Database) GetAllPosts() ([]*Post, error) {
	ids, err := db.client.SMembers(redisContext, db.toKey("posts")).Result()
	if err != nil {
		return nil, err
	}
	posts := []*Post{}
	errs := []string{}
	for _, id := range ids {
		post, err := db.GetPost(id)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			posts = append(posts, post)
		}
	}
	if len(errs) == 0 {
		return posts, nil
	} else {
		return posts, errors.New(strings.Join(errs, "\n"))
	}
}

func (db *Database) Report() (string, error) {
	times, err := db.client.SMembers(redisContext, db.toKey("times")).Result()
	if err != nil {
		return "", err
	}

	report := []string{}
	var totalSize int64 = 0
	for _, t := range times {
		size, err := db.client.SCard(redisContext, db.toKey("time", t)).Result()
		if err != nil {
			return "", err
		}
		if t == TimeIsNotSpecified {
			t = "--:--"
		}
		report = append(report, fmt.Sprintf("%s  -  %d", t, size))
		totalSize += size
	}

	return strings.Join(report, "\n"), nil
}

func (db *Database) Size() int64 {
	size, _ := db.client.SCard(redisContext, db.toKey("posts")).Result()
	return size
}

func IsErrRedisNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "redis: nil")
}

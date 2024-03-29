package bot

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/apex/log"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
	"github.com/sol-armada/admin/cache"
	"github.com/sol-armada/admin/config"
	"github.com/sol-armada/admin/health"
	"github.com/sol-armada/admin/ranks"
	"github.com/sol-armada/admin/rsi"
	"github.com/sol-armada/admin/stores"
	"github.com/sol-armada/admin/users"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/exp/slices"
)

func (b *Bot) UserMonitor(stop <-chan bool, done chan bool) {
	logger := log.WithField("func", "UserMonitor")
	logger.Info("monitoring discord for users")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	lastChecked := time.Now().Add(-30 * time.Minute)
	d := false
	for {
		select {
		case <-stop:
			logger.Info("stopping monitor")
			d = true
			goto DONE
		case <-ticker.C:
			if !health.IsHealthy() {
				time.Sleep(10 * time.Second)
				continue
			}
			if time.Now().After(lastChecked.Add(30 * time.Minute)) {
				logger.Info("scanning users")
				if !stores.Connected() {
					logger.Debug("storage not setup, waiting a bit")
					time.Sleep(10 * time.Second)
					continue
				}

				// rate limit protection
				rateBucket := b.Ratelimiter.GetBucket("guild_member_check")
				if rateBucket.Remaining == 0 {
					logger.Warn("hit a rate limit. relaxing until it goes away")
					time.Sleep(b.Ratelimiter.GetWaitTime(rateBucket, 0))
					continue
				}

				// get the discord members
				m, err := b.GetMembers()
				if err != nil {
					logger.WithError(err).Error("bot getting members")
					continue
				}

				// actually do the members update
				if err := updateMembers(m); err != nil {
					if strings.Contains(err.Error(), "Forbidden") {
						lastChecked = time.Now()
						continue
					}

					logger.WithError(err).Error("updating members")
					continue
				}

				// get the stored members
				storedUsers := []*users.User{}
				// cur, err := stores.Users.List(bson.M{"updated": bson.M{"$lte": time.Now().Add(-30 * time.Minute).UTC()}})
				// if err != nil {
				// 	logger.WithError(err).Error("getting users for updating")
				// 	continue
				// }
				// if err := cur.All(context.Background(), &storedUsers); err != nil {
				// 	logger.WithError(err).Error("getting users from collection for update")
				// 	continue
				// }
				rawUsers := cache.Cache.GetUsers()
				for _, v := range rawUsers {
					uByte, _ := json.Marshal(v)
					u := users.User{}
					if err := json.Unmarshal(uByte, &u); err != nil {
						logger.WithError(err).Error("unmarshalling user from cache")
						continue
					}
					storedUsers = append(storedUsers, &u)
				}

				// do some cleaning
				if err := cleanMembers(m, storedUsers); err != nil {
					logger.WithError(err).Error("cleaning up the members")
					continue
				}

				lastChecked = time.Now()
			}

			continue
		}
	DONE:
		if d {
			done <- true
			return
		}
	}
}

func (b *Bot) UpdateMember() error {
	return nil
}

func (b *Bot) GetMembers() ([]*discordgo.Member, error) {
	members, err := b.GuildMembers(b.GuildId, "", 1000)
	if err != nil {
		return nil, errors.Wrap(err, "getting guild members")
	}

	return members, nil
}

func (b *Bot) GetMember(id string) (*discordgo.Member, error) {
	member, err := b.GuildMember(b.GuildId, id)
	if err != nil {
		return nil, errors.Wrap(err, "getting guild member")
	}

	return member, nil
}

func updateMembers(m []*discordgo.Member) error {
	logger := log.WithField("func", "updateMembers")
	logger.WithFields(log.Fields{
		"discord_members": len(m),
	}).Debug("checking users")

	logger.Debugf("updating %d members", len(m))
	for _, member := range m {
		time.Sleep(1 * time.Second)
		mlogger := logger.WithField("member", member)
		mlogger.Debug("updating member")

		// get the stord user, if we have one
		u, err := users.Get(member.User.ID)
		if err != nil && !errors.Is(err, users.UserNotFound) {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				mlogger.WithError(err).Error("getting member for update")
				continue
			}

			u = users.New(member)
		}
		if u == nil {
			u = users.New(member)
		}
		u.Discord = member
		u.Name = strings.ReplaceAll(u.GetTrueNick(), ".", "")
		u.RSIMember = true

		// rsi related stuff
		u, err = rsi.GetOrgInfo(u)
		if err != nil {
			if strings.Contains(err.Error(), "Forbidden") || strings.Contains(err.Error(), "Bad Gateway") {
				return err
			}

			if !errors.Is(err, rsi.UserNotFound) {
				return errors.Wrap(err, "getting rsi based rank")
			}

			mlogger.WithField("user", u).Debug("user not found")
			u.RSIMember = false
		}

		if u.RSIMember {
			u.BadAffiliation = false
			u.IsAlly = false

			for _, affiliatedOrg := range u.Affilations {
				if slices.Contains(config.GetStringSlice("enemies"), affiliatedOrg) {
					u.BadAffiliation = true
					break
				}
			}
			for _, ally := range config.GetStringSlice("allies") {
				if strings.EqualFold(u.PrimaryOrg, ally) {
					u.IsAlly = true
					break
				}
			}
		}

		// discord related stuff
		u.Avatar = member.Avatar
		if slices.Contains(member.Roles, config.GetString("DISCORD.ROLE_IDS.RECRUIT")) {
			mlogger.Debug("is recruit")
			u.Rank = ranks.Recruit
		}
		if member.User.Bot {
			u.IsBot = true
		}

		// fill legacy
		u.LegacyEvents = u.Events

		if err := u.Save(); err != nil {
			return err
		}
	}

	return nil
}

func cleanMembers(m []*discordgo.Member, storedUsers []*users.User) error {
	for _, user := range storedUsers {
		for _, member := range m {
			if user.ID == member.User.ID {
				goto CONTINUE
			}
		}

		log.WithField("user", user).Info("deleting user")
		if err := user.Delete(); err != nil {
			return errors.Wrap(err, "cleaning members")
		}
	CONTINUE:
		continue
	}

	return nil
}

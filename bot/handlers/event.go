package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/apex/log"
	"github.com/bwmarrin/discordgo"
	"github.com/sol-armada/admin/config"
)

var eventSubCommands = map[string]func(*discordgo.Session, *discordgo.Interaction){
	"attendance": takeAttendance,
}

var activeEvent *discordgo.Message

func EventCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if handler, ok := eventSubCommands[i.ApplicationCommandData().Options[0].Name]; ok {
		handler(s, i.Interaction)
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "That sub command doesn't exist. Not sure how you even got here. Good job.",
		},
	}); err != nil {
		log.WithError(err).Error("responding to event command interaction")
	}
}

func takeAttendance(s *discordgo.Session, i *discordgo.Interaction) {
	g, err := s.State.Guild(i.GuildID)
	if err != nil {
		log.WithError(err).Error("getting guild state")
		return
	}

	if len(g.VoiceStates) == 0 {
		if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "No one is in voice chats!",
			},
		}); err != nil {
			log.WithError(err).Error("responding to take attendance command interaction")
		}
		return
	}

	// get the configured attendance channel id, otherwise use the channel we are in
	attendanceChannel, err := s.Channel(config.GetStringWithDefault("discord.channels.attendance", i.ChannelID))
	if err != nil {
		log.WithError(err).Error("getting attendance channel")
		return
	}

	if activeEvent != nil {
		content := "There is already an event being tracked."
		if i.ChannelID != attendanceChannel.ID {
			content += fmt.Sprintf(" Check %s", attendanceChannel.Mention())
		}
		if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: content,
			},
		}); err != nil {
			log.WithError(err).Error("responding to take attendance command interaction")
		}
		return
	}

	buttons := []discordgo.MessageComponent{}
	for _, vs := range g.VoiceStates {
		label := vs.Member.User.Username
		if vs.Member.Nick != "" {
			label = vs.Member.Nick
		}
		buttons = append(buttons, discordgo.Button{
			Label:    label,
			CustomID: "event:attendance:toggle:" + vs.Member.User.ID,
			Style:    discordgo.PrimaryButton,
		})
	}

	content := "Taking attendance..."
	if i.ChannelID != attendanceChannel.ID {
		content += fmt.Sprintf(" check out %s", attendanceChannel.Mention())
	}
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: content,
		},
	}); err != nil {
		log.WithError(err).Error("responding to take attendance command interaction")
	}

	m, err := s.ChannelMessageSendComplex(attendanceChannel.ID, &discordgo.MessageSend{
		Content: "Click any member to toggle their attendance.\n:blue_square: attended    :red_square: not attended",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: buttons,
			},
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Submit",
						Style:    discordgo.SuccessButton,
						CustomID: "event:attendance:submit",
					},
				},
			},
		},
	})
	if err != nil {
		log.WithError(err).Error("sending message to channel for attendance command")
	}

	activeEvent = m
}

func EventInteractionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if strings.HasPrefix(i.MessageComponentData().CustomID, "event:attendance:toggle:") {
		toggleAttendance(s, i.Interaction)
		return
	}
	if i.MessageComponentData().CustomID == "event:attendance:submit" {
		submitAttendance(s, i.Interaction)
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "That sub command doesn't exist. Not sure how you even got here. Good job.",
		},
	}); err != nil {
		log.WithError(err).Error("responding to event command interaction")
	}
}

func toggleAttendance(s *discordgo.Session, i *discordgo.Interaction) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.WithError(err).Error("responding to toggle attendance interaction")
		return
	}

	memberButtonToToggle := i.MessageComponentData().CustomID

	for index, component := range i.Message.Components[0].(*discordgo.ActionsRow).Components {
		if component.Type() == discordgo.ButtonComponent {
			c := component.(*discordgo.Button)
			if c.CustomID == memberButtonToToggle {
				if c.Style == discordgo.PrimaryButton {
					c.Style = discordgo.DangerButton
				} else {
					c.Style = discordgo.PrimaryButton
				}

				i.Message.Components[0].(*discordgo.ActionsRow).Components[index] = c
				break
			}
		}
	}

	if _, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         i.Message.ID,
		Channel:    i.Message.ChannelID,
		Components: i.Message.Components,
	}); err != nil {
		log.WithError(err).Error("editing original attendance message")
	}
}

func submitAttendance(s *discordgo.Session, i *discordgo.Interaction) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.WithError(err).Error("responding to submit attendance interaction")
	}

	// get the configured attendance channel id, otherwise use the channel we are in
	attendanceChannel, err := s.Channel(config.GetStringWithDefault("discord.channels.attendance", i.ChannelID))
	if err != nil {
		log.WithError(err).Error("getting attendance channel")
		return
	}

	attendies := ""
	for _, button := range i.Message.Components[0].(*discordgo.ActionsRow).Components {
		b := button.(*discordgo.Button)
		if b.Style == discordgo.PrimaryButton {
			attendies += b.Label + "\n"
		}
	}
	if _, err := s.ChannelMessageSendComplex(attendanceChannel.ID, &discordgo.MessageSend{
		Content: fmt.Sprintf("%s\n%s", time.Now().Format("**Jan 02**"), strings.TrimRight(attendies, "\n")),
	}); err != nil {
		log.WithError(err).Error("sending attendance sumbittion message")
		return
	}

	if err := s.ChannelMessageDelete(activeEvent.ChannelID, activeEvent.ID); err != nil {
		log.WithError(err).Error("deleting original attendance message")
	}

	activeEvent = nil
}
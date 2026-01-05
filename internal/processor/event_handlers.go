package processor

import (
	"context"
	"fmt"
	"time"
)

func (ep *EventProcessor) HandleUserUpdate(ctx context.Context, event Event) error {
	userData, ok := event.Data["user"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid user data in USER_UPDATE")
	}

	userID, _ := userData["id"].(string)
	if userID == "" {
		return fmt.Errorf("missing user id")
	}

	// Extract fields
	var username, discriminator, globalName, avatarHash, bio *string
	if v, ok := userData["username"].(string); ok && v != "" {
		username = &v
	}
	if v, ok := userData["discriminator"].(string); ok && v != "" {
		discriminator = &v
	}
	if v, ok := userData["global_name"].(string); ok && v != "" {
		globalName = &v
	}
	if v, ok := userData["avatar"].(string); ok && v != "" {
		avatarHash = &v
	}
	if v, ok := userData["bio"].(string); ok && v != "" {
		bio = &v
	}

	// Extract connected_accounts
	var connectedAccounts []map[string]interface{}
	if accounts, ok := userData["connected_accounts"].([]interface{}); ok {
		for _, acc := range accounts {
			if accMap, ok := acc.(map[string]interface{}); ok {
				connectedAccounts = append(connectedAccounts, accMap)
			}
		}
	}

	// Ensure user exists
	_, err := ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)
	if err != nil {
		return err
	}

	// Check for changes and insert into history tables
	if username != nil || globalName != nil || discriminator != nil {
		if err := ep.handleUsernameChange(ctx, userID, username, discriminator, globalName); err != nil {
			ep.log.Warn("failed_to_handle_username_change", "user_id", userID, "error", err)
		}
	}

	if avatarHash != nil {
		if err := ep.handleAvatarChange(ctx, userID, *avatarHash); err != nil {
			ep.log.Warn("failed_to_handle_avatar_change", "user_id", userID, "error", err)
		}
	}

	if bio != nil {
		if err := ep.handleBioChange(ctx, userID, *bio); err != nil {
			ep.log.Warn("failed_to_handle_bio_change", "user_id", userID, "error", err)
		}
	}

	// Handle connected accounts
	for _, acc := range connectedAccounts {
		if err := ep.handleConnectedAccount(ctx, userID, acc); err != nil {
			ep.log.Warn("failed_to_handle_connected_account", "user_id", userID, "error", err)
		}
	}

	// Update last_updated_at
	_, _ = ep.db.Pool.Exec(ctx,
		`UPDATE users SET last_updated_at = NOW() WHERE id = $1`,
		userID,
	)

	return nil
}

func (ep *EventProcessor) HandleGuildMemberUpdate(ctx context.Context, event Event) error {
	userData, ok := event.Data["user"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid user data in GUILD_MEMBER_UPDATE")
	}

	userID, _ := userData["id"].(string)
	if userID == "" {
		return fmt.Errorf("missing user id")
	}

	guildID, _ := event.Data["guild_id"].(string)

	// Extract user fields
	var username, discriminator, globalName, avatarHash *string
	if v, ok := userData["username"].(string); ok && v != "" {
		username = &v
	}
	if v, ok := userData["discriminator"].(string); ok && v != "" {
		discriminator = &v
	}
	if v, ok := userData["global_name"].(string); ok && v != "" {
		globalName = &v
	}
	if v, ok := userData["avatar"].(string); ok && v != "" {
		avatarHash = &v
	}

	// Extract nickname do servidor (campo direto do evento, nao do user)
	var nickname *string
	if v, ok := event.Data["nick"].(string); ok && v != "" {
		nickname = &v
	}

	// Ensure user exists
	_, err := ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)
	if err != nil {
		return err
	}

	// salvar relacao guild_members
	if guildID != "" && event.TokenID > 0 {
		_, _ = ep.db.Pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, token_id, discovered_at, last_seen_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET last_seen_at = NOW()`,
			guildID, userID, event.TokenID,
		)
	}

	// salvar historico de nickname por servidor (com deduplicacao)
	if guildID != "" {
		var lastNick *string
		_ = ep.db.Pool.QueryRow(ctx,
			`SELECT nickname FROM nickname_history 
			 WHERE user_id = $1 AND guild_id = $2 
			 ORDER BY changed_at DESC LIMIT 1`,
			userID, guildID,
		).Scan(&lastNick)

		// comparar nicknames (ambos podem ser nil)
		nicksEqual := (nickname == nil && lastNick == nil) ||
			(nickname != nil && lastNick != nil && *nickname == *lastNick)

		if !nicksEqual {
			_, _ = ep.db.Pool.Exec(ctx,
				`INSERT INTO nickname_history (user_id, guild_id, nickname, changed_at)
				 VALUES ($1, $2, $3, NOW())`,
				userID, guildID, nickname,
			)
		}
	}

	if username != nil || globalName != nil || discriminator != nil {
		if err := ep.handleUsernameChange(ctx, userID, username, discriminator, globalName); err != nil {
			ep.log.Warn("failed_to_handle_username_change", "user_id", userID, "error", err)
		}
	}

	if avatarHash != nil {
		if err := ep.handleAvatarChange(ctx, userID, *avatarHash); err != nil {
			ep.log.Warn("failed_to_handle_avatar_change", "user_id", userID, "error", err)
		}
	}

	// processar dados extras do usuario
	ep.processUserExtras(ctx, userData, userID)

	return nil
}

func (ep *EventProcessor) HandlePresenceUpdate(ctx context.Context, event Event) error {
	userData, ok := event.Data["user"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid user data in PRESENCE_UPDATE")
	}

	userID, _ := userData["id"].(string)
	if userID == "" {
		return fmt.Errorf("missing user id")
	}

	guildID, _ := event.Data["guild_id"].(string)
	status, _ := event.Data["status"].(string) // online, offline, idle, dnd

	// Extract user profile fields from PRESENCE_UPDATE
	var username, discriminator, globalName, avatarHash *string
	if v, ok := userData["username"].(string); ok && v != "" {
		username = &v
	}
	if v, ok := userData["discriminator"].(string); ok && v != "" {
		discriminator = &v
	}
	if v, ok := userData["global_name"].(string); ok && v != "" {
		globalName = &v
	}
	if v, ok := userData["avatar"].(string); ok && v != "" {
		avatarHash = &v
	}

	// garantir que usuario existe
	_, _ = ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)

	// mark user as freshly observed
	_, _ = ep.db.Pool.Exec(ctx,
		`UPDATE users SET last_updated_at = NOW() WHERE id = $1`,
		userID,
	)

	// salvar relacao guild_members (usuario online/offline no servidor)
	if guildID != "" && event.TokenID > 0 {
		_, _ = ep.db.Pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, token_id, discovered_at, last_seen_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET last_seen_at = NOW()`,
			guildID, userID, event.TokenID,
		)
	}

	// Save username/global_name history (from PRESENCE_UPDATE user data)
	if username != nil || globalName != nil || discriminator != nil {
		ep.log.Debug("presence_user_data_extracted",
			"user_id", userID,
			"username", username,
			"global_name", globalName,
			"discriminator", discriminator,
		)
		if err := ep.handleUsernameChange(ctx, userID, username, discriminator, globalName); err != nil {
			ep.log.Warn("failed_to_handle_username_change_presence", "user_id", userID, "error", err)
		}
	}

	// Save avatar history (from PRESENCE_UPDATE user data)
	if avatarHash != nil {
		ep.log.Debug("presence_avatar_extracted",
			"user_id", userID,
			"avatar_hash", *avatarHash,
		)
		if err := ep.handleAvatarChange(ctx, userID, *avatarHash); err != nil {
			ep.log.Warn("failed_to_handle_avatar_change_presence", "user_id", userID, "error", err)
		}
	}

	// salvar historico de status/presenca (com deduplicacao)
	if status != "" {
		var lastStatus string
		_ = ep.db.Pool.QueryRow(ctx,
			`SELECT status FROM presence_history 
			 WHERE user_id = $1 AND (guild_id = $2 OR (guild_id IS NULL AND $2 IS NULL))
			 ORDER BY changed_at DESC LIMIT 1`,
			userID, guildID,
		).Scan(&lastStatus)

		if lastStatus != status {
			_, _ = ep.db.Pool.Exec(ctx,
				`INSERT INTO presence_history (user_id, guild_id, status, changed_at)
				 VALUES ($1, $2, $3, NOW())`,
				userID, guildID, status,
			)
		}
	}

	// Extract bio if available
	var bio *string
	if v, ok := userData["bio"].(string); ok && v != "" {
		bio = &v
	}

	// Extract activities e salvar historico
	activities, ok := event.Data["activities"].([]interface{})
	if ok {
		for _, act := range activities {
			if actMap, ok := act.(map[string]interface{}); ok {
				actType, _ := actMap["type"].(float64)
				actName, _ := actMap["name"].(string)
				actDetails, _ := actMap["details"].(string)
				actState, _ := actMap["state"].(string)
				actURL, _ := actMap["url"].(string)
				appID, _ := actMap["application_id"].(string)

				// salvar atividade no historico (com deduplicacao por nome)
				if actName != "" {
					var exists bool
					_ = ep.db.Pool.QueryRow(ctx,
						`SELECT EXISTS(
							SELECT 1 FROM activity_history 
							WHERE user_id = $1 AND name = $2 AND ended_at IS NULL
							LIMIT 1
						)`,
						userID, actName,
					).Scan(&exists)

					if !exists {
						// spotify especial
						var spotifyTrack, spotifyArtist, spotifyAlbum *string
						if actType == 2 { // Listening
							if syncID, ok := actMap["sync_id"].(string); ok && syncID != "" {
								spotifyTrack = &syncID
								// salvar connected account do spotify
								acc := map[string]interface{}{
									"type":        "spotify",
									"external_id": syncID,
									"name":        "Spotify",
								}
								ep.handleConnectedAccount(ctx, userID, acc)
							}
							if assets, ok := actMap["assets"].(map[string]interface{}); ok {
								if largeText, ok := assets["large_text"].(string); ok {
									spotifyAlbum = &largeText
								}
							}
							if state, ok := actMap["state"].(string); ok {
								spotifyArtist = &state
							}
						}

						_, _ = ep.db.Pool.Exec(ctx,
							`INSERT INTO activity_history 
							 (user_id, activity_type, name, details, state, url, application_id, started_at, spotify_track_id, spotify_artist, spotify_album)
							 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8, $9, $10)`,
							userID, int(actType), actName, actDetails, actState, actURL, appID,
							spotifyTrack, spotifyArtist, spotifyAlbum,
						)
					}
				}
			}
		}

		// finalizar atividades que nao estao mais ativas
		if len(activities) == 0 {
			_, _ = ep.db.Pool.Exec(ctx,
				`UPDATE activity_history SET ended_at = NOW() 
				 WHERE user_id = $1 AND ended_at IS NULL`,
				userID,
			)
		}
	}

	if bio != nil {
		if err := ep.handleBioChange(ctx, userID, *bio); err != nil {
			ep.log.Warn("failed_to_handle_bio_change", "user_id", userID, "error", err)
		}
	}

	return nil
}

func (ep *EventProcessor) HandleGuildMembersChunk(ctx context.Context, event Event) error {
	members, ok := event.Data["members"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid members data in GUILD_MEMBERS_CHUNK")
	}

	guildID, _ := event.Data["guild_id"].(string)
	if guildID == "" {
		return fmt.Errorf("missing guild_id")
	}

	for _, member := range members {
		memberMap, ok := member.(map[string]interface{})
		if !ok {
			continue
		}

		userData, ok := memberMap["user"].(map[string]interface{})
		if !ok {
			continue
		}

		userID, _ := userData["id"].(string)
		if userID == "" {
			continue
		}

		// Extract all fields
		var username, discriminator, globalName, avatarHash, bio *string
		if v, ok := userData["username"].(string); ok && v != "" {
			username = &v
		}
		if v, ok := userData["discriminator"].(string); ok && v != "" {
			discriminator = &v
		}
		if v, ok := userData["global_name"].(string); ok && v != "" {
			globalName = &v
		}
		if v, ok := userData["avatar"].(string); ok && v != "" {
			avatarHash = &v
		}
		if v, ok := userData["bio"].(string); ok && v != "" {
			bio = &v
		}

		// Ensure user exists
		_, _ = ep.db.Pool.Exec(ctx,
			`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
			userID,
		)

		// Insert current state into history (with deduplication check)
		if username != nil || globalName != nil || discriminator != nil {
			ep.handleUsernameChange(ctx, userID, username, discriminator, globalName)
		}

		if avatarHash != nil {
			ep.handleAvatarChange(ctx, userID, *avatarHash)
		}

		if bio != nil {
			ep.handleBioChange(ctx, userID, *bio)
		}

		// Handle connected accounts
		if accounts, ok := userData["connected_accounts"].([]interface{}); ok {
			for _, acc := range accounts {
				if accMap, ok := acc.(map[string]interface{}); ok {
					ep.handleConnectedAccount(ctx, userID, accMap)
				}
			}
		}
	}

	// Some Discord chunk payloads include presence snapshots under `presences`.
	// When available, process them as if they were PRESENCE_UPDATE to populate presence/activity history.
	if presencesRaw, ok := event.Data["presences"].([]interface{}); ok && len(presencesRaw) > 0 {
		for _, pr := range presencesRaw {
			presenceMap, ok := pr.(map[string]interface{})
			if !ok {
				continue
			}
			if _, hasGuild := presenceMap["guild_id"]; !hasGuild {
				presenceMap["guild_id"] = guildID
			}
			_ = ep.HandlePresenceUpdate(ctx, Event{
				Type:      "PRESENCE_UPDATE",
				Data:      presenceMap,
				Timestamp: event.Timestamp,
				TokenID:   event.TokenID,
			})
		}
	}

	return nil
}

// Helper functions

func (ep *EventProcessor) handleUsernameChange(ctx context.Context, userID string, username, discriminator, globalName *string) error {
	// Check if this exact combination already exists
	var exists bool
	err := ep.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM username_history 
			WHERE user_id = $1 AND username IS NOT DISTINCT FROM $2 
			AND discriminator IS NOT DISTINCT FROM $3 
			AND global_name IS NOT DISTINCT FROM $4
			LIMIT 1
		)`,
		userID, username, discriminator, globalName,
	).Scan(&exists)

	if err == nil && !exists {
		_, err = ep.db.Pool.Exec(ctx,
			`INSERT INTO username_history (user_id, username, discriminator, global_name, changed_at)
			 VALUES ($1, $2, $3, $4, NOW())`,
			userID, username, discriminator, globalName,
		)
	}

	return err
}

func (ep *EventProcessor) handleAvatarChange(ctx context.Context, userID, avatarHash string) error {
	// Check if this avatar already exists
	var exists bool
	err := ep.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM avatar_history 
			WHERE user_id = $1 AND hash_avatar = $2
			LIMIT 1
		)`,
		userID, avatarHash,
	).Scan(&exists)

	if err == nil && !exists {
		// Download and upload avatar (async, will be handled by storage client)
		// For now, just insert with NULL cdn_url
		_, err = ep.db.Pool.Exec(ctx,
			`INSERT INTO avatar_history (user_id, hash_avatar, url_cdn, changed_at)
			 VALUES ($1, $2, NULL, NOW())`,
			userID, avatarHash,
		)
	}

	return err
}

func (ep *EventProcessor) handleBioChange(ctx context.Context, userID, bio string) error {
	// Check if this bio already exists
	var exists bool
	err := ep.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM bio_history 
			WHERE user_id = $1 AND bio_content = $2
			LIMIT 1
		)`,
		userID, bio,
	).Scan(&exists)

	if err == nil && !exists {
		_, err = ep.db.Pool.Exec(ctx,
			`INSERT INTO bio_history (user_id, bio_content, changed_at)
			 VALUES ($1, $2, NOW())`,
			userID, bio,
		)
	}

	return err
}

func (ep *EventProcessor) handleConnectedAccount(ctx context.Context, userID string, account map[string]interface{}) error {
	accType, _ := account["type"].(string)
	externalID, _ := account["id"].(string)
	name, _ := account["name"].(string)

	if accType == "" {
		return nil
	}

	// garantir que o usuario existe antes de tentar salvar connected_account
	_, err := ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)
	if err != nil {
		ep.log.Warn("failed_to_ensure_user_exists", "user_id", userID, "error", err)
		return err
	}

	// Check if exists, then insert or update
	var existingID int64
	err = ep.db.Pool.QueryRow(ctx,
		`SELECT id FROM connected_accounts 
		 WHERE user_id = $1 AND type = $2 AND (external_id = $3 OR (external_id IS NULL AND $3 IS NULL))
		 LIMIT 1`,
		userID, accType, externalID,
	).Scan(&existingID)

	if err != nil {
		// Insert new
		_, err = ep.db.Pool.Exec(ctx,
			`INSERT INTO connected_accounts (user_id, type, external_id, name, observed_at, last_seen_at)
			 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
			userID, accType, externalID, name,
		)
	} else {
		// Update existing
		_, err = ep.db.Pool.Exec(ctx,
			`UPDATE connected_accounts 
			 SET last_seen_at = NOW(), name = $1 
			 WHERE id = $2`,
			name, existingID,
		)
	}

	return err
}

// HandleMessageCreate captura usuarios de mensagens no chat e estatisticas
func (ep *EventProcessor) HandleMessageCreate(ctx context.Context, event Event) error {
	// capturar autor da mensagem
	authorData, ok := event.Data["author"].(map[string]interface{})
	if !ok {
		return nil
	}

	userID, _ := authorData["id"].(string)
	if userID == "" {
		return nil
	}

	guildID, _ := event.Data["guild_id"].(string)
	channelID, _ := event.Data["channel_id"].(string)
	messageID, _ := event.Data["id"].(string)
	content, _ := event.Data["content"].(string)
	timestamp, _ := event.Data["timestamp"].(string)
	editedTimestamp, _ := event.Data["edited_timestamp"].(string)

	// extrair dados do autor
	var username, discriminator, globalName, avatarHash *string
	if v, ok := authorData["username"].(string); ok && v != "" {
		username = &v
	}
	if v, ok := authorData["discriminator"].(string); ok && v != "" {
		discriminator = &v
	}
	if v, ok := authorData["global_name"].(string); ok && v != "" {
		globalName = &v
	}
	if v, ok := authorData["avatar"].(string); ok && v != "" {
		avatarHash = &v
	}

	// garantir que usuario existe
	_, _ = ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)

	// salvar relacao guild_members se temos guild e token
	if guildID != "" && event.TokenID > 0 {
		_, _ = ep.db.Pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, token_id, discovered_at, last_seen_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET last_seen_at = NOW()`,
			guildID, userID, event.TokenID,
		)
	}

	// salvar estatisticas de mensagens
	if guildID != "" && channelID != "" {
		if _, err := ep.db.Pool.Exec(ctx,
			`INSERT INTO message_stats (user_id, guild_id, channel_id, message_count, first_message_at, last_message_at)
			 VALUES ($1, $2, $3, 1, NOW(), NOW())
			 ON CONFLICT (user_id, guild_id, channel_id) DO UPDATE SET 
				message_count = message_stats.message_count + 1,
				last_message_at = NOW()`,
			userID, guildID, channelID,
		); err != nil {
			ep.log.Warn("message_stats_insert_failed", "user_id", userID, "guild_id", guildID, "channel_id", channelID, "error", err)
		}
	}

	// salvar mensagem completa se tiver ID
	if messageID != "" && channelID != "" {
		createdAt := time.Now().UTC()
		if timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
				createdAt = t
			} else if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
				createdAt = t
			} else {
				ep.log.Warn("message_timestamp_parse_failed", "user_id", userID, "message_id", messageID, "timestamp", timestamp, "error", err)
			}
		}

		var editedAt *time.Time
		if editedTimestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, editedTimestamp); err == nil {
				editedAt = &t
			} else if t, err := time.Parse(time.RFC3339, editedTimestamp); err == nil {
				editedAt = &t
			}
		}

		hasAttachments := false
		hasEmbeds := false
		var replyToMsgID, replyToUserID *string

		if attachments, ok := event.Data["attachments"].([]interface{}); ok && len(attachments) > 0 {
			hasAttachments = true
		}
		if embeds, ok := event.Data["embeds"].([]interface{}); ok && len(embeds) > 0 {
			hasEmbeds = true
		}

		// capturar reply
		if ref, ok := event.Data["message_reference"].(map[string]interface{}); ok {
			if refMsgID, ok := ref["message_id"].(string); ok {
				replyToMsgID = &refMsgID
			}
		}
		if refMsg, ok := event.Data["referenced_message"].(map[string]interface{}); ok {
			if refAuthor, ok := refMsg["author"].(map[string]interface{}); ok {
				if refUserID, ok := refAuthor["id"].(string); ok {
					replyToUserID = &refUserID
				}
			}
		}

		if _, err := ep.db.Pool.Exec(ctx,
			`INSERT INTO messages (message_id, user_id, guild_id, channel_id, content, created_at, edited_at, has_attachments, has_embeds, reply_to_message_id, reply_to_user_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 ON CONFLICT (message_id) DO NOTHING`,
			messageID, userID, guildID, channelID, content, createdAt, editedAt, hasAttachments, hasEmbeds, replyToMsgID, replyToUserID,
		); err != nil {
			ep.log.Warn("message_insert_failed", "user_id", userID, "message_id", messageID, "guild_id", guildID, "channel_id", channelID, "error", err)
		}
	}

	// salvar username se tiver
	if username != nil || globalName != nil || discriminator != nil {
		ep.handleUsernameChange(ctx, userID, username, discriminator, globalName)
	}

	// salvar avatar se tiver
	if avatarHash != nil {
		ep.handleAvatarChange(ctx, userID, *avatarHash)
	}

	// processar dados extras do autor
	ep.processUserExtras(ctx, authorData, userID)

	// capturar mencoes tambem
	if mentions, ok := event.Data["mentions"].([]interface{}); ok {
		for _, mention := range mentions {
			if mentionData, ok := mention.(map[string]interface{}); ok {
				ep.processUserFromData(ctx, mentionData, guildID, event.TokenID)
			}
		}
	}

	// capturar membro referenciado (se for reply)
	if referencedMessage, ok := event.Data["referenced_message"].(map[string]interface{}); ok {
		if refAuthor, ok := referencedMessage["author"].(map[string]interface{}); ok {
			ep.processUserFromData(ctx, refAuthor, guildID, event.TokenID)
		}
	}

	return nil
}

// HandleVoiceStateUpdate captura usuarios em call e salva sessoes de voz
func (ep *EventProcessor) HandleVoiceStateUpdate(ctx context.Context, event Event) error {
	userID, _ := event.Data["user_id"].(string)
	if userID == "" {
		return nil
	}

	guildID, _ := event.Data["guild_id"].(string)
	channelID, _ := event.Data["channel_id"].(string)

	// garantir que usuario existe
	_, _ = ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)

	// salvar relacao guild_members
	if guildID != "" && event.TokenID > 0 {
		_, _ = ep.db.Pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, token_id, discovered_at, last_seen_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET last_seen_at = NOW()`,
			guildID, userID, event.TokenID,
		)
	}

	// extrair flags de voz
	selfMute, _ := event.Data["self_mute"].(bool)
	selfDeaf, _ := event.Data["self_deaf"].(bool)
	selfStream, _ := event.Data["self_stream"].(bool)
	selfVideo, _ := event.Data["self_video"].(bool)

	if channelID != "" && guildID != "" {
		// Verificar se já existe sessão ativa para este usuário neste canal
		var existingSessionID int64
		err := ep.db.Pool.QueryRow(ctx,
			`SELECT id FROM voice_sessions 
			 WHERE user_id = $1 AND guild_id = $2 AND channel_id = $3 AND left_at IS NULL 
			 LIMIT 1`,
			userID, guildID, channelID,
		).Scan(&existingSessionID)

		if err == nil && existingSessionID > 0 {
			// Sessão já existe - apenas atualizar flags (mute/deaf/video/stream podem mudar durante a call)
			_, _ = ep.db.Pool.Exec(ctx,
				`UPDATE voice_sessions SET 
					was_muted = was_muted OR $2,
					was_deafened = was_deafened OR $3,
					was_streaming = was_streaming OR $4,
					was_video = was_video OR $5
				 WHERE id = $1`,
				existingSessionID, selfMute, selfDeaf, selfStream, selfVideo,
			)
			// Não criar nova sessão, apenas retornar após processar member data
		} else {
			// Não existe sessão ativa - criar nova sessão (usuário acabou de entrar)
			var sessionID int64
			err := ep.db.Pool.QueryRow(ctx,
				`INSERT INTO voice_sessions (user_id, guild_id, channel_id, joined_at, was_muted, was_deafened, was_streaming, was_video)
				 VALUES ($1, $2, $3, NOW(), $4, $5, $6, $7)
				 RETURNING id`,
				userID, guildID, channelID, selfMute, selfDeaf, selfStream, selfVideo,
			).Scan(&sessionID)

			if err == nil && sessionID > 0 {
				// buscar outros usuarios no mesmo canal e registrar como participantes
				rows, _ := ep.db.Pool.Query(ctx,
					`SELECT DISTINCT user_id FROM voice_sessions 
					 WHERE guild_id = $1 AND channel_id = $2 AND left_at IS NULL AND user_id != $3`,
					guildID, channelID, userID,
				)
				if rows != nil {
					defer rows.Close()
					for rows.Next() {
						var partnerID string
						if rows.Scan(&partnerID) == nil && partnerID != "" {
							// registrar participante na sessao
							_, _ = ep.db.Pool.Exec(ctx,
								`INSERT INTO voice_participants (session_id, user_id, guild_id, channel_id, joined_at)
								 VALUES ($1, $2, $3, $4, NOW())`,
								sessionID, partnerID, guildID, channelID,
							)

							// atualizar estatisticas de parceiros (bidirecional)
							_, _ = ep.db.Pool.Exec(ctx,
								`INSERT INTO voice_partner_stats (user_id, partner_id, guild_id, total_sessions, last_call_at)
								 VALUES ($1, $2, $3, 1, NOW())
								 ON CONFLICT (user_id, partner_id, guild_id) DO UPDATE SET 
									total_sessions = voice_partner_stats.total_sessions + 1,
									last_call_at = NOW()`,
								userID, partnerID, guildID,
							)
							_, _ = ep.db.Pool.Exec(ctx,
								`INSERT INTO voice_partner_stats (user_id, partner_id, guild_id, total_sessions, last_call_at)
								 VALUES ($1, $2, $3, 1, NOW())
								 ON CONFLICT (user_id, partner_id, guild_id) DO UPDATE SET 
									total_sessions = voice_partner_stats.total_sessions + 1,
									last_call_at = NOW()`,
								partnerID, userID, guildID,
							)
						}
					}
				}

				// atualizar estatisticas apenas quando cria nova sessão
				_, _ = ep.db.Pool.Exec(ctx,
					`INSERT INTO voice_stats (user_id, guild_id, total_sessions, last_session_at)
					 VALUES ($1, $2, 1, NOW())
					 ON CONFLICT (user_id, guild_id) DO UPDATE SET 
						total_sessions = voice_stats.total_sessions + 1,
						last_session_at = NOW()`,
					userID, guildID,
				)
			}
		}
	} else if channelID == "" && guildID != "" {
		// usuario saiu do canal de voz - finalizar sessao
		var sessionID int64
		var oldChannelID string
		_ = ep.db.Pool.QueryRow(ctx,
			`SELECT id, channel_id FROM voice_sessions 
			 WHERE user_id = $1 AND guild_id = $2 AND left_at IS NULL 
			 ORDER BY joined_at DESC LIMIT 1`,
			userID, guildID,
		).Scan(&sessionID, &oldChannelID)

		_, _ = ep.db.Pool.Exec(ctx,
			`UPDATE voice_sessions 
			 SET left_at = NOW(), 
				 duration_seconds = EXTRACT(EPOCH FROM (NOW() - joined_at))::INTEGER
			 WHERE user_id = $1 AND guild_id = $2 AND left_at IS NULL`,
			userID, guildID,
		)

		// marcar participantes como saiu
		if sessionID > 0 {
			_, _ = ep.db.Pool.Exec(ctx,
				`UPDATE voice_participants SET left_at = NOW() 
				 WHERE session_id = $1 AND left_at IS NULL`,
				sessionID,
			)

			// atualizar duracao dos parceiros
			if oldChannelID != "" {
				rows, _ := ep.db.Pool.Query(ctx,
					`SELECT DISTINCT user_id FROM voice_participants 
					 WHERE session_id = $1`,
					sessionID,
				)
				if rows != nil {
					defer rows.Close()
					for rows.Next() {
						var partnerID string
						if rows.Scan(&partnerID) == nil && partnerID != "" {
							// calcular duracao desta sessao
							var duration int64
							_ = ep.db.Pool.QueryRow(ctx,
								`SELECT COALESCE(duration_seconds, 0) FROM voice_sessions WHERE id = $1`,
								sessionID,
							).Scan(&duration)

							if duration > 0 {
								_, _ = ep.db.Pool.Exec(ctx,
									`UPDATE voice_partner_stats 
									 SET total_duration_seconds = total_duration_seconds + $4
									 WHERE user_id = $1 AND partner_id = $2 AND guild_id = $3`,
									userID, partnerID, guildID, duration,
								)
								_, _ = ep.db.Pool.Exec(ctx,
									`UPDATE voice_partner_stats 
									 SET total_duration_seconds = total_duration_seconds + $4
									 WHERE user_id = $1 AND partner_id = $2 AND guild_id = $3`,
									partnerID, userID, guildID, duration,
								)
							}
						}
					}
				}
			}
		}

		// atualizar duracao total nas estatisticas
		_, _ = ep.db.Pool.Exec(ctx,
			`UPDATE voice_stats 
			 SET total_duration_seconds = total_duration_seconds + COALESCE(
				(SELECT duration_seconds FROM voice_sessions 
				 WHERE user_id = $1 AND guild_id = $2 
				 ORDER BY left_at DESC LIMIT 1), 0)
			 WHERE user_id = $1 AND guild_id = $2`,
			userID, guildID,
		)
	}

	// se tiver dados do membro, processar
	if memberData, ok := event.Data["member"].(map[string]interface{}); ok {
		if userData, ok := memberData["user"].(map[string]interface{}); ok {
			ep.processUserFromData(ctx, userData, guildID, event.TokenID)
		}
	}

	return nil
}

// HandleTypingStart captura usuarios digitando
func (ep *EventProcessor) HandleTypingStart(ctx context.Context, event Event) error {
	userID, _ := event.Data["user_id"].(string)
	if userID == "" {
		return nil
	}

	guildID, _ := event.Data["guild_id"].(string)

	// garantir que usuario existe
	_, _ = ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)

	// salvar relacao guild_members
	if guildID != "" && event.TokenID > 0 {
		_, _ = ep.db.Pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, token_id, discovered_at, last_seen_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET last_seen_at = NOW()`,
			guildID, userID, event.TokenID,
		)
	}

	// se tiver dados do membro, processar
	if memberData, ok := event.Data["member"].(map[string]interface{}); ok {
		if userData, ok := memberData["user"].(map[string]interface{}); ok {
			ep.processUserFromData(ctx, userData, guildID, event.TokenID)
		}
	}

	return nil
}

// HandleGuildMemberAdd captura novos membros entrando no servidor
func (ep *EventProcessor) HandleGuildMemberAdd(ctx context.Context, event Event) error {
	userData, ok := event.Data["user"].(map[string]interface{})
	if !ok {
		return nil
	}

	userID, _ := userData["id"].(string)
	if userID == "" {
		return nil
	}

	guildID, _ := event.Data["guild_id"].(string)

	// processar dados do usuario
	ep.processUserFromData(ctx, userData, guildID, event.TokenID)

	return nil
}

// processUserFromData processa dados de usuario de qualquer evento
func (ep *EventProcessor) processUserFromData(ctx context.Context, userData map[string]interface{}, guildID string, tokenID int64) {
	userID, _ := userData["id"].(string)
	if userID == "" {
		return
	}

	// extrair campos
	var username, discriminator, globalName, avatarHash, bio *string
	if v, ok := userData["username"].(string); ok && v != "" {
		username = &v
	}
	if v, ok := userData["discriminator"].(string); ok && v != "" {
		discriminator = &v
	}
	if v, ok := userData["global_name"].(string); ok && v != "" {
		globalName = &v
	}
	if v, ok := userData["avatar"].(string); ok && v != "" {
		avatarHash = &v
	}
	if v, ok := userData["bio"].(string); ok && v != "" {
		bio = &v
	}

	// garantir que usuario existe
	_, _ = ep.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)

	// salvar relacao guild_members
	if guildID != "" && tokenID > 0 {
		_, _ = ep.db.Pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, token_id, discovered_at, last_seen_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET last_seen_at = NOW()`,
			guildID, userID, tokenID,
		)
	}

	// salvar dados
	if username != nil || globalName != nil || discriminator != nil {
		ep.handleUsernameChange(ctx, userID, username, discriminator, globalName)
	}
	if avatarHash != nil {
		ep.handleAvatarChange(ctx, userID, *avatarHash)
	}
	if bio != nil {
		ep.handleBioChange(ctx, userID, *bio)
	}

	// connected accounts
	if accounts, ok := userData["connected_accounts"].([]interface{}); ok {
		for _, acc := range accounts {
			if accMap, ok := acc.(map[string]interface{}); ok {
				ep.handleConnectedAccount(ctx, userID, accMap)
			}
		}
	}

	// processar dados extras (banner, decoration, clan, etc)
	ep.processUserExtras(ctx, userData, userID)
}

// processUserExtras processa dados extras do usuario (banner, decoration, clan, flags, etc)
func (ep *EventProcessor) processUserExtras(ctx context.Context, userData map[string]interface{}, userID string) {
	// banner
	if bannerHash, ok := userData["banner"].(string); ok && bannerHash != "" {
		var accentColor *string
		if color, ok := userData["accent_color"].(float64); ok {
			colorHex := fmt.Sprintf("#%06x", int(color))
			accentColor = &colorHex
		}

		// verificar se ja existe
		var exists bool
		_ = ep.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM banner_history 
				WHERE user_id = $1 AND banner_hash = $2
				LIMIT 1
			)`,
			userID, bannerHash,
		).Scan(&exists)

		if !exists {
			_, _ = ep.db.Pool.Exec(ctx,
				`INSERT INTO banner_history (user_id, banner_hash, banner_color, changed_at)
				 VALUES ($1, $2, $3, NOW())`,
				userID, bannerHash, accentColor,
			)
		}
	}

	// avatar decoration
	if decoration, ok := userData["avatar_decoration_data"].(map[string]interface{}); ok {
		asset, _ := decoration["asset"].(string)
		skuID, _ := decoration["sku_id"].(string)

		if asset != "" {
			var exists bool
			_ = ep.db.Pool.QueryRow(ctx,
				`SELECT EXISTS(
					SELECT 1 FROM avatar_decoration_history 
					WHERE user_id = $1 AND decoration_asset = $2
					LIMIT 1
				)`,
				userID, asset,
			).Scan(&exists)

			if !exists {
				_, _ = ep.db.Pool.Exec(ctx,
					`INSERT INTO avatar_decoration_history (user_id, decoration_asset, decoration_sku_id, changed_at)
					 VALUES ($1, $2, $3, NOW())`,
					userID, asset, skuID,
				)
			}
		}
	}

	// clan
	if clan, ok := userData["clan"].(map[string]interface{}); ok {
		tag, _ := clan["tag"].(string)
		identityGuildID, _ := clan["identity_guild_id"].(string)
		badge, _ := clan["badge"].(string)

		if tag != "" || identityGuildID != "" {
			var exists bool
			_ = ep.db.Pool.QueryRow(ctx,
				`SELECT EXISTS(
					SELECT 1 FROM clan_history 
					WHERE user_id = $1 AND clan_tag IS NOT DISTINCT FROM $2 
					AND clan_identity_guild_id IS NOT DISTINCT FROM $3
					LIMIT 1
				)`,
				userID, tag, identityGuildID,
			).Scan(&exists)

			if !exists {
				_, _ = ep.db.Pool.Exec(ctx,
					`INSERT INTO clan_history (user_id, clan_tag, clan_identity_guild_id, badge, changed_at)
					 VALUES ($1, $2, $3, $4, NOW())`,
					userID, tag, identityGuildID, badge,
				)
			}
		}
	}

	// atualizar campos extras na tabela users
	var accentColor, premiumType, publicFlags, flags *int
	var bot, system, mfaEnabled, verified *bool
	var locale, email *string

	if v, ok := userData["accent_color"].(float64); ok {
		val := int(v)
		accentColor = &val
	}
	if v, ok := userData["premium_type"].(float64); ok {
		val := int(v)
		premiumType = &val
	}
	if v, ok := userData["public_flags"].(float64); ok {
		val := int(v)
		publicFlags = &val
	}
	if v, ok := userData["flags"].(float64); ok {
		val := int(v)
		flags = &val
	}
	if v, ok := userData["bot"].(bool); ok {
		bot = &v
	}
	if v, ok := userData["system"].(bool); ok {
		system = &v
	}
	if v, ok := userData["mfa_enabled"].(bool); ok {
		mfaEnabled = &v
	}
	if v, ok := userData["verified"].(bool); ok {
		verified = &v
	}
	if v, ok := userData["locale"].(string); ok && v != "" {
		locale = &v
	}
	if v, ok := userData["email"].(string); ok && v != "" {
		email = &v
	}

	// atualizar campos se houver algum dado
	if accentColor != nil || premiumType != nil || publicFlags != nil || flags != nil ||
		bot != nil || system != nil || mfaEnabled != nil || verified != nil || locale != nil || email != nil {
		_, _ = ep.db.Pool.Exec(ctx,
			`UPDATE users SET 
				accent_color = COALESCE($2, accent_color),
				premium_type = COALESCE($3, premium_type),
				public_flags = COALESCE($4, public_flags),
				flags = COALESCE($5, flags),
				bot = COALESCE($6, bot),
				is_system = COALESCE($7, is_system),
				mfa_enabled = COALESCE($8, mfa_enabled),
				verified = COALESCE($9, verified),
				locale = COALESCE($10, locale),
				email = COALESCE($11, email),
				last_updated_at = NOW()
			 WHERE id = $1`,
			userID, accentColor, premiumType, publicFlags, flags, bot, system, mfaEnabled, verified, locale, email,
		)
	}
}

// HandleGuildCreate processa eventos GUILD_CREATE para salvar member_count e dados do guild
func (ep *EventProcessor) HandleGuildCreate(ctx context.Context, event Event) error {
	guildID, _ := event.Data["id"].(string)
	if guildID == "" {
		return fmt.Errorf("missing guild_id in GUILD_CREATE")
	}

	// Extrair dados do guild
	name, _ := event.Data["name"].(string)
	icon, _ := event.Data["icon"].(string)

	// member_count é enviado como float64 pelo JSON
	var memberCount int
	if mc, ok := event.Data["member_count"].(float64); ok {
		memberCount = int(mc)
	}

	// Salvar/atualizar guild com member_count
	_, err := ep.db.Pool.Exec(ctx,
		`INSERT INTO guilds (guild_id, name, icon, member_count, discovered_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (guild_id) DO UPDATE SET 
			name = COALESCE(NULLIF($2, ''), guilds.name),
			icon = COALESCE($3, guilds.icon),
			member_count = CASE WHEN $4 > 0 THEN $4 ELSE guilds.member_count END`,
		guildID, name, icon, memberCount,
	)
	if err != nil {
		ep.log.Warn("failed_to_save_guild", "guild_id", guildID, "error", err)
		return err
	}

	if memberCount > 0 {
		ep.log.Info("guild_create_with_member_count",
			"guild_id", guildID,
			"name", name,
			"member_count", memberCount,
		)
	}

	return nil
}

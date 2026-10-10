-- +goose Up

PRAGMA foreign_keys = ON;

-- =============================================================================
-- 1. Identity and Sessions
-- =============================================================================

CREATE TABLE users (
    user_id                  INTEGER PRIMARY KEY,
    uuid                     TEXT    NOT NULL UNIQUE,
    player_id                INTEGER NOT NULL DEFAULT 0,
    os_type                  INTEGER NOT NULL DEFAULT 0,
    platform_type            INTEGER NOT NULL DEFAULT 0,
    user_restriction_type    INTEGER NOT NULL DEFAULT 0,
    register_datetime        INTEGER NOT NULL DEFAULT 0,
    game_start_datetime      INTEGER NOT NULL DEFAULT 0,
    latest_version           INTEGER NOT NULL DEFAULT 0,
    birth_year               INTEGER NOT NULL DEFAULT 0,
    birth_month              INTEGER NOT NULL DEFAULT 0,
    backup_token             TEXT    NOT NULL DEFAULT '',
    charge_money_this_month  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE sessions (
    session_key  TEXT    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(user_id),
    uuid         TEXT    NOT NULL,
    expire_at    TEXT    NOT NULL
);

-- =============================================================================
-- 1b. Per-User 1:1 State Tables
-- =============================================================================

CREATE TABLE user_setting (
    user_id                   INTEGER PRIMARY KEY REFERENCES users(user_id),
    is_notify_purchase_alert  INTEGER NOT NULL DEFAULT 0,
    latest_version            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_status (
    user_id                  INTEGER PRIMARY KEY REFERENCES users(user_id),
    level                    INTEGER NOT NULL DEFAULT 0,
    exp                      INTEGER NOT NULL DEFAULT 0,
    stamina_milli_value      INTEGER NOT NULL DEFAULT 0,
    stamina_update_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version           INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_gem (
    user_id   INTEGER PRIMARY KEY REFERENCES users(user_id),
    paid_gem  INTEGER NOT NULL DEFAULT 0,
    free_gem  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_profile (
    user_id                              INTEGER PRIMARY KEY REFERENCES users(user_id),
    name                                 TEXT    NOT NULL DEFAULT '',
    name_update_datetime                 INTEGER NOT NULL DEFAULT 0,
    message                              TEXT    NOT NULL DEFAULT '',
    message_update_datetime              INTEGER NOT NULL DEFAULT 0,
    favorite_costume_id                  INTEGER NOT NULL DEFAULT 0,
    favorite_costume_id_update_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version                       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_login (
    user_id                       INTEGER PRIMARY KEY REFERENCES users(user_id),
    total_login_count             INTEGER NOT NULL DEFAULT 0,
    continual_login_count         INTEGER NOT NULL DEFAULT 0,
    max_continual_login_count     INTEGER NOT NULL DEFAULT 0,
    last_login_datetime           INTEGER NOT NULL DEFAULT 0,
    last_comeback_login_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version                INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_login_bonus (
    user_id                         INTEGER PRIMARY KEY REFERENCES users(user_id),
    login_bonus_id                  INTEGER NOT NULL DEFAULT 0,
    current_page_number             INTEGER NOT NULL DEFAULT 0,
    current_stamp_number            INTEGER NOT NULL DEFAULT 0,
    latest_reward_receive_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version                  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_main_quest (
    user_id                             INTEGER PRIMARY KEY REFERENCES users(user_id),
    current_quest_flow_type             INTEGER NOT NULL DEFAULT 0,
    current_main_quest_route_id         INTEGER NOT NULL DEFAULT 0,
    current_quest_scene_id              INTEGER NOT NULL DEFAULT 0,
    head_quest_scene_id                 INTEGER NOT NULL DEFAULT 0,
    is_reached_last_quest_scene         INTEGER NOT NULL DEFAULT 0,
    progress_quest_scene_id             INTEGER NOT NULL DEFAULT 0,
    progress_head_quest_scene_id        INTEGER NOT NULL DEFAULT 0,
    progress_quest_flow_type            INTEGER NOT NULL DEFAULT 0,
    main_quest_season_id                INTEGER NOT NULL DEFAULT 0,
    latest_version                      INTEGER NOT NULL DEFAULT 0,
    saved_current_quest_scene_id        INTEGER NOT NULL DEFAULT 0,
    saved_head_quest_scene_id           INTEGER NOT NULL DEFAULT 0,
    replay_flow_current_quest_scene_id  INTEGER NOT NULL DEFAULT 0,
    replay_flow_head_quest_scene_id     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_event_quest (
    user_id                         INTEGER PRIMARY KEY REFERENCES users(user_id),
    current_event_quest_chapter_id  INTEGER NOT NULL DEFAULT 0,
    current_quest_id                INTEGER NOT NULL DEFAULT 0,
    current_quest_scene_id          INTEGER NOT NULL DEFAULT 0,
    head_quest_scene_id             INTEGER NOT NULL DEFAULT 0,
    latest_version                  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_extra_quest (
    user_id                 INTEGER PRIMARY KEY REFERENCES users(user_id),
    current_quest_id        INTEGER NOT NULL DEFAULT 0,
    current_quest_scene_id  INTEGER NOT NULL DEFAULT 0,
    head_quest_scene_id     INTEGER NOT NULL DEFAULT 0,
    latest_version          INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_side_story_active (
    user_id                            INTEGER PRIMARY KEY REFERENCES users(user_id),
    current_side_story_quest_id        INTEGER NOT NULL DEFAULT 0,
    current_side_story_quest_scene_id  INTEGER NOT NULL DEFAULT 0,
    latest_version                     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_big_hunt_state (
    user_id                         INTEGER PRIMARY KEY REFERENCES users(user_id),
    current_big_hunt_boss_quest_id  INTEGER NOT NULL DEFAULT 0,
    current_big_hunt_quest_id       INTEGER NOT NULL DEFAULT 0,
    current_quest_scene_id          INTEGER NOT NULL DEFAULT 0,
    is_dry_run                      INTEGER NOT NULL DEFAULT 0,
    latest_version                  INTEGER NOT NULL DEFAULT 0,
    deck_type                       INTEGER NOT NULL DEFAULT 0,
    user_triple_deck_number         INTEGER NOT NULL DEFAULT 0,
    boss_knock_down_count           INTEGER NOT NULL DEFAULT 0,
    max_combo_count                 INTEGER NOT NULL DEFAULT 0,
    total_damage                    INTEGER NOT NULL DEFAULT 0,
    deck_number                     INTEGER NOT NULL DEFAULT 0,
    battle_binary                   BLOB
);

CREATE TABLE user_battle (
    user_id                           INTEGER PRIMARY KEY REFERENCES users(user_id),
    is_active                         INTEGER NOT NULL DEFAULT 0,
    start_count                       INTEGER NOT NULL DEFAULT 0,
    finish_count                      INTEGER NOT NULL DEFAULT 0,
    last_started_at                   INTEGER NOT NULL DEFAULT 0,
    last_finished_at                  INTEGER NOT NULL DEFAULT 0,
    last_user_party_count             INTEGER NOT NULL DEFAULT 0,
    last_npc_party_count              INTEGER NOT NULL DEFAULT 0,
    last_battle_binary_size           INTEGER NOT NULL DEFAULT 0,
    last_elapsed_frame_count          INTEGER NOT NULL DEFAULT 0,
    last_character_death_count        INTEGER NOT NULL DEFAULT 0,
    last_costume_skill_used_count     INTEGER NOT NULL DEFAULT 0,
    last_weapon_skill_used_count      INTEGER NOT NULL DEFAULT 0,
    last_companion_skill_used_count   INTEGER NOT NULL DEFAULT 0,
    last_critical_count               INTEGER NOT NULL DEFAULT 0,
    last_combo_count                  INTEGER NOT NULL DEFAULT 0,
    last_combo_max_damage             INTEGER NOT NULL DEFAULT 0,
    last_costume_alive_count          INTEGER NOT NULL DEFAULT 0,
    last_costume_party_size           INTEGER NOT NULL DEFAULT 3,
    last_costume_hp_percent_0         INTEGER NOT NULL DEFAULT 0,
    last_costume_hp_percent_1         INTEGER NOT NULL DEFAULT 0,
    last_costume_hp_percent_2         INTEGER NOT NULL DEFAULT 0,
    last_total_recover_point          INTEGER NOT NULL DEFAULT 0,
    last_party_character_id_0         INTEGER NOT NULL DEFAULT 0,
    last_party_character_id_1         INTEGER NOT NULL DEFAULT 0,
    last_party_character_id_2         INTEGER NOT NULL DEFAULT 0,
    last_main_weapon_id_0             INTEGER NOT NULL DEFAULT 0,
    last_main_weapon_id_1             INTEGER NOT NULL DEFAULT 0,
    last_main_weapon_id_2             INTEGER NOT NULL DEFAULT 0,
    last_started_quest_id             INTEGER NOT NULL DEFAULT 0,
    last_started_quest_at             INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_notification (
    user_id                       INTEGER PRIMARY KEY REFERENCES users(user_id),
    gift_not_receive_count        INTEGER NOT NULL DEFAULT 0,
    friend_request_receive_count  INTEGER NOT NULL DEFAULT 0,
    is_exist_unread_information   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_portal_cage (
    user_id                   INTEGER PRIMARY KEY REFERENCES users(user_id),
    is_current_progress       INTEGER NOT NULL DEFAULT 0,
    drop_item_start_datetime  INTEGER NOT NULL DEFAULT 0,
    current_drop_item_count   INTEGER NOT NULL DEFAULT 0,
    latest_version            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_guerrilla_free_open (
    user_id             INTEGER PRIMARY KEY REFERENCES users(user_id),
    start_datetime      INTEGER NOT NULL DEFAULT 0,
    open_minutes        INTEGER NOT NULL DEFAULT 0,
    daily_opened_count  INTEGER NOT NULL DEFAULT 0,
    latest_version      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_explore (
    user_id                INTEGER PRIMARY KEY REFERENCES users(user_id),
    is_use_explore_ticket  INTEGER NOT NULL DEFAULT 0,
    playing_explore_id     INTEGER NOT NULL DEFAULT 0,
    latest_play_datetime   INTEGER NOT NULL DEFAULT 0,
    latest_version         INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_shop_replaceable (
    user_id                        INTEGER PRIMARY KEY REFERENCES users(user_id),
    lineup_update_count            INTEGER NOT NULL DEFAULT 0,
    latest_lineup_update_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version                 INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_gacha (
    user_id                    INTEGER PRIMARY KEY REFERENCES users(user_id),
    reward_available           INTEGER NOT NULL DEFAULT 0,
    todays_current_draw_count  INTEGER NOT NULL DEFAULT 0,
    daily_max_count            INTEGER NOT NULL DEFAULT 0,
    last_reward_draw_date      INTEGER NOT NULL DEFAULT 0,
    obtain_consumable_item_id  INTEGER,
    obtain_count               INTEGER
);

-- =============================================================================
-- 2. Characters and Progression
-- =============================================================================

CREATE TABLE user_characters (
    user_id         INTEGER NOT NULL REFERENCES users(user_id),
    character_id    INTEGER NOT NULL,
    level           INTEGER NOT NULL DEFAULT 0,
    exp             INTEGER NOT NULL DEFAULT 0,
    latest_version  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_id)
);

CREATE TABLE user_character_boards (
    user_id             INTEGER NOT NULL REFERENCES users(user_id),
    character_board_id  INTEGER NOT NULL,
    panel_release_bit1  INTEGER NOT NULL DEFAULT 0,
    panel_release_bit2  INTEGER NOT NULL DEFAULT 0,
    panel_release_bit3  INTEGER NOT NULL DEFAULT 0,
    panel_release_bit4  INTEGER NOT NULL DEFAULT 0,
    latest_version      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_board_id)
);

CREATE TABLE user_character_board_abilities (
    user_id         INTEGER NOT NULL REFERENCES users(user_id),
    character_id    INTEGER NOT NULL,
    ability_id      INTEGER NOT NULL,
    level           INTEGER NOT NULL DEFAULT 0,
    latest_version  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_id, ability_id)
);

CREATE TABLE user_character_board_status_ups (
    user_id                  INTEGER NOT NULL REFERENCES users(user_id),
    character_id             INTEGER NOT NULL,
    status_calculation_type  INTEGER NOT NULL,
    hp                       INTEGER NOT NULL DEFAULT 0,
    attack                   INTEGER NOT NULL DEFAULT 0,
    vitality                 INTEGER NOT NULL DEFAULT 0,
    agility                  INTEGER NOT NULL DEFAULT 0,
    critical_ratio           INTEGER NOT NULL DEFAULT 0,
    critical_attack          INTEGER NOT NULL DEFAULT 0,
    latest_version           INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_id, status_calculation_type)
);

CREATE TABLE user_character_rebirths (
    user_id         INTEGER NOT NULL REFERENCES users(user_id),
    character_id    INTEGER NOT NULL,
    rebirth_count   INTEGER NOT NULL DEFAULT 0,
    latest_version  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_id)
);

CREATE TABLE user_character_viewer_fields (
    user_id                     INTEGER NOT NULL REFERENCES users(user_id),
    character_viewer_field_id   INTEGER NOT NULL,
    is_viewed                   INTEGER NOT NULL DEFAULT 0,
    viewed_datetime             INTEGER NOT NULL DEFAULT 0,
    latest_version              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_viewer_field_id)
);

-- =============================================================================
-- 3. Equipment (UUID-keyed)
-- =============================================================================

CREATE TABLE user_costumes (
    user_id                 INTEGER NOT NULL REFERENCES users(user_id),
    user_costume_uuid       TEXT    NOT NULL,
    costume_id              INTEGER NOT NULL,
    limit_break_count       INTEGER NOT NULL DEFAULT 0,
    level                   INTEGER NOT NULL DEFAULT 0,
    exp                     INTEGER NOT NULL DEFAULT 0,
    headup_display_view_id  INTEGER NOT NULL DEFAULT 0,
    acquisition_datetime    INTEGER NOT NULL DEFAULT 0,
    awaken_count            INTEGER NOT NULL DEFAULT 0,
    latest_version          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_costume_uuid)
);

CREATE TABLE user_costume_active_skills (
    user_id               INTEGER NOT NULL REFERENCES users(user_id),
    user_costume_uuid     TEXT    NOT NULL,
    level                 INTEGER NOT NULL DEFAULT 0,
    acquisition_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_costume_uuid)
);

CREATE TABLE user_costume_awaken_status_ups (
    user_id                  INTEGER NOT NULL REFERENCES users(user_id),
    user_costume_uuid        TEXT    NOT NULL,
    status_calculation_type  INTEGER NOT NULL,
    hp                       INTEGER NOT NULL DEFAULT 0,
    attack                   INTEGER NOT NULL DEFAULT 0,
    vitality                 INTEGER NOT NULL DEFAULT 0,
    agility                  INTEGER NOT NULL DEFAULT 0,
    critical_ratio           INTEGER NOT NULL DEFAULT 0,
    critical_attack          INTEGER NOT NULL DEFAULT 0,
    latest_version           INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_costume_uuid, status_calculation_type)
);

CREATE TABLE user_weapons (
    user_id               INTEGER NOT NULL REFERENCES users(user_id),
    user_weapon_uuid      TEXT    NOT NULL,
    weapon_id             INTEGER NOT NULL,
    level                 INTEGER NOT NULL DEFAULT 0,
    exp                   INTEGER NOT NULL DEFAULT 0,
    limit_break_count     INTEGER NOT NULL DEFAULT 0,
    is_protected          INTEGER NOT NULL DEFAULT 0,
    acquisition_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_weapon_uuid)
);

CREATE TABLE user_weapon_skills (
    user_id           INTEGER NOT NULL REFERENCES users(user_id),
    user_weapon_uuid  TEXT    NOT NULL,
    slot_number       INTEGER NOT NULL,
    level             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_weapon_uuid, slot_number)
);

CREATE TABLE user_weapon_abilities (
    user_id           INTEGER NOT NULL REFERENCES users(user_id),
    user_weapon_uuid  TEXT    NOT NULL,
    slot_number       INTEGER NOT NULL,
    level             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_weapon_uuid, slot_number)
);

CREATE TABLE user_weapon_awakens (
    user_id           INTEGER NOT NULL REFERENCES users(user_id),
    user_weapon_uuid  TEXT    NOT NULL,
    latest_version    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_weapon_uuid)
);

CREATE TABLE user_weapon_stories (
    user_id                   INTEGER NOT NULL REFERENCES users(user_id),
    weapon_id                 INTEGER NOT NULL,
    released_max_story_index  INTEGER NOT NULL DEFAULT 0,
    latest_version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, weapon_id)
);

CREATE TABLE user_weapon_notes (
    user_id                     INTEGER NOT NULL REFERENCES users(user_id),
    weapon_id                   INTEGER NOT NULL,
    max_level                   INTEGER NOT NULL DEFAULT 0,
    max_limit_break_count       INTEGER NOT NULL DEFAULT 0,
    first_acquisition_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, weapon_id)
);

CREATE TABLE user_companions (
    user_id                 INTEGER NOT NULL REFERENCES users(user_id),
    user_companion_uuid     TEXT    NOT NULL,
    companion_id            INTEGER NOT NULL,
    headup_display_view_id  INTEGER NOT NULL DEFAULT 0,
    level                   INTEGER NOT NULL DEFAULT 0,
    acquisition_datetime    INTEGER NOT NULL DEFAULT 0,
    latest_version          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_companion_uuid)
);

CREATE TABLE user_thoughts (
    user_id               INTEGER NOT NULL REFERENCES users(user_id),
    user_thought_uuid     TEXT    NOT NULL,
    thought_id            INTEGER NOT NULL,
    acquisition_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_thought_uuid)
);

CREATE TABLE user_parts (
    user_id               INTEGER NOT NULL REFERENCES users(user_id),
    user_parts_uuid       TEXT    NOT NULL,
    parts_id              INTEGER NOT NULL,
    level                 INTEGER NOT NULL DEFAULT 0,
    parts_status_main_id  INTEGER NOT NULL DEFAULT 0,
    is_protected          INTEGER NOT NULL DEFAULT 0,
    acquisition_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_parts_uuid)
);

CREATE TABLE user_parts_group_notes (
    user_id                     INTEGER NOT NULL REFERENCES users(user_id),
    parts_group_id              INTEGER NOT NULL,
    first_acquisition_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, parts_group_id)
);

CREATE TABLE user_parts_presets (
    user_id                       INTEGER NOT NULL REFERENCES users(user_id),
    user_parts_preset_number      INTEGER NOT NULL,
    user_parts_uuid01             TEXT    NOT NULL DEFAULT '',
    user_parts_uuid02             TEXT    NOT NULL DEFAULT '',
    user_parts_uuid03             TEXT    NOT NULL DEFAULT '',
    name                          TEXT    NOT NULL DEFAULT '',
    user_parts_preset_tag_number  INTEGER NOT NULL DEFAULT 0,
    latest_version                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_parts_preset_number)
);

-- =============================================================================
-- 4. Deck System
-- =============================================================================

CREATE TABLE user_deck_characters (
    user_id                   INTEGER NOT NULL REFERENCES users(user_id),
    user_deck_character_uuid  TEXT    NOT NULL,
    user_costume_uuid         TEXT    NOT NULL DEFAULT '',
    main_user_weapon_uuid     TEXT    NOT NULL DEFAULT '',
    user_companion_uuid       TEXT    NOT NULL DEFAULT '',
    power                     INTEGER NOT NULL DEFAULT 0,
    user_thought_uuid         TEXT    NOT NULL DEFAULT '',
    dressup_costume_id        INTEGER NOT NULL DEFAULT 0,
    latest_version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_deck_character_uuid)
);

CREATE TABLE user_decks (
    user_id                     INTEGER NOT NULL REFERENCES users(user_id),
    deck_type                   INTEGER NOT NULL,
    user_deck_number            INTEGER NOT NULL,
    user_deck_character_uuid01  TEXT    NOT NULL DEFAULT '',
    user_deck_character_uuid02  TEXT    NOT NULL DEFAULT '',
    user_deck_character_uuid03  TEXT    NOT NULL DEFAULT '',
    name                        TEXT    NOT NULL DEFAULT '',
    power                       INTEGER NOT NULL DEFAULT 0,
    latest_version              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, deck_type, user_deck_number)
);

CREATE TABLE user_deck_sub_weapons (
    user_id                   INTEGER NOT NULL REFERENCES users(user_id),
    user_deck_character_uuid  TEXT    NOT NULL,
    ordinal                   INTEGER NOT NULL,
    user_weapon_uuid          TEXT    NOT NULL,
    PRIMARY KEY (user_id, user_deck_character_uuid, ordinal)
);

CREATE TABLE user_deck_parts (
    user_id                   INTEGER NOT NULL REFERENCES users(user_id),
    user_deck_character_uuid  TEXT    NOT NULL,
    ordinal                   INTEGER NOT NULL,
    user_parts_uuid           TEXT    NOT NULL,
    PRIMARY KEY (user_id, user_deck_character_uuid, ordinal)
);

CREATE TABLE user_deck_type_notes (
    user_id         INTEGER NOT NULL REFERENCES users(user_id),
    deck_type       INTEGER NOT NULL,
    max_deck_power  INTEGER NOT NULL DEFAULT 0,
    latest_version  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, deck_type)
);

-- =============================================================================
-- 5. Quests
-- =============================================================================

CREATE TABLE user_quests (
    user_id                INTEGER NOT NULL REFERENCES users(user_id),
    quest_id               INTEGER NOT NULL,
    quest_state_type       INTEGER NOT NULL DEFAULT 0,
    is_battle_only         INTEGER NOT NULL DEFAULT 0,
    user_deck_number       INTEGER NOT NULL DEFAULT 0,
    latest_start_datetime  INTEGER NOT NULL DEFAULT 0,
    clear_count            INTEGER NOT NULL DEFAULT 0,
    daily_clear_count      INTEGER NOT NULL DEFAULT 0,
    last_clear_datetime    INTEGER NOT NULL DEFAULT 0,
    shortest_clear_frames  INTEGER NOT NULL DEFAULT 0,
    is_reward_granted      INTEGER NOT NULL DEFAULT 0,
    latest_version         INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, quest_id)
);

CREATE TABLE user_quest_missions (
    user_id                INTEGER NOT NULL REFERENCES users(user_id),
    quest_id               INTEGER NOT NULL,
    quest_mission_id       INTEGER NOT NULL,
    progress_value         INTEGER NOT NULL DEFAULT 0,
    is_clear               INTEGER NOT NULL DEFAULT 0,
    latest_clear_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version         INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, quest_id, quest_mission_id)
);

CREATE TABLE user_missions (
    user_id                       INTEGER NOT NULL REFERENCES users(user_id),
    mission_id                    INTEGER NOT NULL,
    start_datetime                INTEGER NOT NULL DEFAULT 0,
    progress_value                INTEGER NOT NULL DEFAULT 0,
    mission_progress_status_type  INTEGER NOT NULL DEFAULT 0,
    clear_datetime                INTEGER NOT NULL DEFAULT 0,
    latest_version                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, mission_id)
);

CREATE TABLE user_side_story_quests (
    user_id                         INTEGER NOT NULL REFERENCES users(user_id),
    side_story_quest_id             INTEGER NOT NULL,
    head_side_story_quest_scene_id  INTEGER NOT NULL DEFAULT 0,
    side_story_quest_state_type     INTEGER NOT NULL DEFAULT 0,
    latest_version                  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, side_story_quest_id)
);

CREATE TABLE user_quest_limit_content_status (
    user_id                          INTEGER NOT NULL REFERENCES users(user_id),
    limit_content_id                 INTEGER NOT NULL,
    limit_content_quest_status_type  INTEGER NOT NULL DEFAULT 0,
    event_quest_chapter_id           INTEGER NOT NULL DEFAULT 0,
    latest_version                   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, limit_content_id)
);

-- =============================================================================
-- 6. Big Hunt
-- =============================================================================

CREATE TABLE user_big_hunt_max_scores (
    user_id                    INTEGER NOT NULL REFERENCES users(user_id),
    big_hunt_boss_id           INTEGER NOT NULL,
    max_score                  INTEGER NOT NULL DEFAULT 0,
    max_score_update_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, big_hunt_boss_id)
);

CREATE TABLE user_big_hunt_statuses (
    user_id                    INTEGER NOT NULL REFERENCES users(user_id),
    big_hunt_boss_id           INTEGER NOT NULL,
    daily_challenge_count      INTEGER NOT NULL DEFAULT 0,
    latest_challenge_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, big_hunt_boss_id)
);

CREATE TABLE user_big_hunt_schedule_max_scores (
    user_id                    INTEGER NOT NULL REFERENCES users(user_id),
    big_hunt_schedule_id       INTEGER NOT NULL,
    big_hunt_boss_id           INTEGER NOT NULL,
    max_score                  INTEGER NOT NULL DEFAULT 0,
    max_score_update_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, big_hunt_schedule_id, big_hunt_boss_id)
);

CREATE TABLE user_big_hunt_weekly_max_scores (
    user_id                  INTEGER NOT NULL REFERENCES users(user_id),
    big_hunt_weekly_version  INTEGER NOT NULL,
    attribute_type           INTEGER NOT NULL,
    max_score                INTEGER NOT NULL DEFAULT 0,
    latest_version           INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, big_hunt_weekly_version, attribute_type)
);

CREATE TABLE user_big_hunt_weekly_statuses (
    user_id                    INTEGER NOT NULL REFERENCES users(user_id),
    big_hunt_weekly_version    INTEGER NOT NULL,
    is_received_weekly_reward  INTEGER NOT NULL DEFAULT 0,
    latest_version             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, big_hunt_weekly_version)
);

-- =============================================================================
-- 7. Gimmicks
-- =============================================================================

CREATE TABLE user_gimmick_progress (
    user_id                       INTEGER NOT NULL REFERENCES users(user_id),
    gimmick_sequence_schedule_id  INTEGER NOT NULL,
    gimmick_sequence_id           INTEGER NOT NULL,
    gimmick_id                    INTEGER NOT NULL,
    is_gimmick_cleared            INTEGER NOT NULL DEFAULT 0,
    start_datetime                INTEGER NOT NULL DEFAULT 0,
    latest_version                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, gimmick_sequence_schedule_id, gimmick_sequence_id, gimmick_id)
);

CREATE TABLE user_gimmick_ornament_progress (
    user_id                       INTEGER NOT NULL REFERENCES users(user_id),
    gimmick_sequence_schedule_id  INTEGER NOT NULL,
    gimmick_sequence_id           INTEGER NOT NULL,
    gimmick_id                    INTEGER NOT NULL,
    gimmick_ornament_index        INTEGER NOT NULL,
    progress_value_bit            INTEGER NOT NULL DEFAULT 0,
    base_datetime                 INTEGER NOT NULL DEFAULT 0,
    latest_version                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, gimmick_sequence_schedule_id, gimmick_sequence_id, gimmick_id, gimmick_ornament_index)
);

CREATE TABLE user_gimmick_sequences (
    user_id                       INTEGER NOT NULL REFERENCES users(user_id),
    gimmick_sequence_schedule_id  INTEGER NOT NULL,
    gimmick_sequence_id           INTEGER NOT NULL,
    is_gimmick_sequence_cleared   INTEGER NOT NULL DEFAULT 0,
    clear_datetime                INTEGER NOT NULL DEFAULT 0,
    latest_version                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, gimmick_sequence_schedule_id, gimmick_sequence_id)
);

CREATE TABLE user_gimmick_unlocks (
    user_id                       INTEGER NOT NULL REFERENCES users(user_id),
    gimmick_sequence_schedule_id  INTEGER NOT NULL,
    gimmick_sequence_id           INTEGER NOT NULL,
    gimmick_id                    INTEGER NOT NULL,
    is_unlocked                   INTEGER NOT NULL DEFAULT 0,
    latest_version                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, gimmick_sequence_schedule_id, gimmick_sequence_id, gimmick_id)
);

-- =============================================================================
-- 8. Inventory
-- =============================================================================

CREATE TABLE user_consumable_items (
    user_id             INTEGER NOT NULL REFERENCES users(user_id),
    consumable_item_id  INTEGER NOT NULL,
    count               INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, consumable_item_id)
);

CREATE TABLE user_materials (
    user_id      INTEGER NOT NULL REFERENCES users(user_id),
    material_id  INTEGER NOT NULL,
    count        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, material_id)
);

CREATE TABLE user_important_items (
    user_id            INTEGER NOT NULL REFERENCES users(user_id),
    important_item_id  INTEGER NOT NULL,
    count              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, important_item_id)
);

CREATE TABLE user_premium_items (
    user_id          INTEGER NOT NULL REFERENCES users(user_id),
    premium_item_id  INTEGER NOT NULL,
    count            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, premium_item_id)
);

CREATE TABLE user_tutorials (
    user_id         INTEGER NOT NULL REFERENCES users(user_id),
    tutorial_type   INTEGER NOT NULL,
    progress_phase  INTEGER NOT NULL DEFAULT 0,
    choice_id       INTEGER NOT NULL DEFAULT 0,
    latest_version  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, tutorial_type)
);

CREATE TABLE user_explore_scores (
    user_id                    INTEGER NOT NULL REFERENCES users(user_id),
    explore_id                 INTEGER NOT NULL,
    max_score                  INTEGER NOT NULL DEFAULT 0,
    max_score_update_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, explore_id)
);

CREATE TABLE user_auto_sale_settings (
    user_id                          INTEGER NOT NULL REFERENCES users(user_id),
    possession_auto_sale_item_type   INTEGER NOT NULL,
    possession_auto_sale_item_value  TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, possession_auto_sale_item_type)
);

-- =============================================================================
-- 9. Simple Progress Maps
-- =============================================================================

CREATE TABLE user_navi_cutin_played (
    user_id        INTEGER NOT NULL REFERENCES users(user_id),
    navi_cutin_id  INTEGER NOT NULL,
    PRIMARY KEY (user_id, navi_cutin_id)
);

CREATE TABLE user_viewed_movies (
    user_id    INTEGER NOT NULL REFERENCES users(user_id),
    movie_id   INTEGER NOT NULL,
    timestamp  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, movie_id)
);

CREATE TABLE user_contents_stories (
    user_id            INTEGER NOT NULL REFERENCES users(user_id),
    contents_story_id  INTEGER NOT NULL,
    timestamp          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, contents_story_id)
);

CREATE TABLE user_drawn_omikuji (
    user_id     INTEGER NOT NULL REFERENCES users(user_id),
    omikuji_id  INTEGER NOT NULL,
    timestamp   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, omikuji_id)
);

CREATE TABLE user_dokan_confirmed (
    user_id   INTEGER NOT NULL REFERENCES users(user_id),
    dokan_id  INTEGER NOT NULL,
    PRIMARY KEY (user_id, dokan_id)
);

-- =============================================================================
-- 10. Gifts
-- =============================================================================

CREATE TABLE user_gifts (
    user_id                   INTEGER NOT NULL REFERENCES users(user_id),
    user_gift_uuid            TEXT    NOT NULL,
    is_received               INTEGER NOT NULL DEFAULT 0,
    possession_type           INTEGER NOT NULL DEFAULT 0,
    possession_id             INTEGER NOT NULL DEFAULT 0,
    count                     INTEGER NOT NULL DEFAULT 0,
    grant_datetime            INTEGER NOT NULL DEFAULT 0,
    description_gift_text_id  INTEGER NOT NULL DEFAULT 0,
    equipment_data            BLOB,
    expiration_datetime       INTEGER,
    received_datetime         INTEGER,
    PRIMARY KEY (user_id, user_gift_uuid)
);

-- =============================================================================
-- 11. Gacha
-- =============================================================================

CREATE TABLE user_gacha_converted_medals (
    user_id             INTEGER NOT NULL REFERENCES users(user_id),
    ordinal             INTEGER NOT NULL,
    consumable_item_id  INTEGER NOT NULL,
    count               INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, ordinal)
);

CREATE TABLE user_gacha_banners (
    user_id      INTEGER NOT NULL REFERENCES users(user_id),
    gacha_id     INTEGER NOT NULL,
    medal_count  INTEGER NOT NULL DEFAULT 0,
    step_number  INTEGER NOT NULL DEFAULT 0,
    loop_count   INTEGER NOT NULL DEFAULT 0,
    draw_count   INTEGER NOT NULL DEFAULT 0,
    box_number   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, gacha_id)
);

CREATE TABLE user_gacha_banner_box_drew_counts (
    user_id      INTEGER NOT NULL REFERENCES users(user_id),
    gacha_id     INTEGER NOT NULL,
    box_item_id  INTEGER NOT NULL,
    count        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, gacha_id, box_item_id)
);

-- =============================================================================
-- 12. Shop
-- =============================================================================

CREATE TABLE user_shop_items (
    user_id                               INTEGER NOT NULL REFERENCES users(user_id),
    shop_item_id                          INTEGER NOT NULL,
    bought_count                          INTEGER NOT NULL DEFAULT 0,
    latest_bought_count_changed_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version                        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, shop_item_id)
);

CREATE TABLE user_shop_replaceable_lineup (
    user_id         INTEGER NOT NULL REFERENCES users(user_id),
    slot_number     INTEGER NOT NULL,
    shop_item_id    INTEGER NOT NULL DEFAULT 0,
    latest_version  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, slot_number)
);

-- =============================================================================
-- 13. Cage Ornaments
-- =============================================================================

CREATE TABLE user_cage_ornament_rewards (
    user_id               INTEGER NOT NULL REFERENCES users(user_id),
    cage_ornament_id      INTEGER NOT NULL,
    acquisition_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, cage_ornament_id)
);

-- +goose Down

DROP TABLE IF EXISTS user_cage_ornament_rewards       ;
DROP TABLE IF EXISTS user_shop_replaceable_lineup     ;
DROP TABLE IF EXISTS user_shop_items                  ;
DROP TABLE IF EXISTS user_gacha_banner_box_drew_counts;
DROP TABLE IF EXISTS user_gacha_banners               ;
DROP TABLE IF EXISTS user_gacha_converted_medals      ;
DROP TABLE IF EXISTS user_gifts                       ;
DROP TABLE IF EXISTS user_dokan_confirmed             ;
DROP TABLE IF EXISTS user_drawn_omikuji               ;
DROP TABLE IF EXISTS user_contents_stories            ;
DROP TABLE IF EXISTS user_viewed_movies               ;
DROP TABLE IF EXISTS user_navi_cutin_played           ;
DROP TABLE IF EXISTS user_auto_sale_settings          ;
DROP TABLE IF EXISTS user_explore_scores              ;
DROP TABLE IF EXISTS user_tutorials                   ;
DROP TABLE IF EXISTS user_premium_items               ;
DROP TABLE IF EXISTS user_important_items             ;
DROP TABLE IF EXISTS user_materials                   ;
DROP TABLE IF EXISTS user_consumable_items            ;
DROP TABLE IF EXISTS user_gimmick_unlocks             ;
DROP TABLE IF EXISTS user_gimmick_sequences           ;
DROP TABLE IF EXISTS user_gimmick_ornament_progress   ;
DROP TABLE IF EXISTS user_gimmick_progress            ;
DROP TABLE IF EXISTS user_big_hunt_weekly_statuses    ;
DROP TABLE IF EXISTS user_big_hunt_weekly_max_scores  ;
DROP TABLE IF EXISTS user_big_hunt_schedule_max_scores;
DROP TABLE IF EXISTS user_big_hunt_statuses           ;
DROP TABLE IF EXISTS user_big_hunt_max_scores         ;
DROP TABLE IF EXISTS user_quest_limit_content_status  ;
DROP TABLE IF EXISTS user_side_story_quests           ;
DROP TABLE IF EXISTS user_missions                    ;
DROP TABLE IF EXISTS user_quest_missions              ;
DROP TABLE IF EXISTS user_quests                      ;
DROP TABLE IF EXISTS user_deck_type_notes             ;
DROP TABLE IF EXISTS user_deck_parts                  ;
DROP TABLE IF EXISTS user_deck_sub_weapons            ;
DROP TABLE IF EXISTS user_decks                       ;
DROP TABLE IF EXISTS user_deck_characters             ;
DROP TABLE IF EXISTS user_parts_presets               ;
DROP TABLE IF EXISTS user_parts_group_notes           ;
DROP TABLE IF EXISTS user_parts                       ;
DROP TABLE IF EXISTS user_thoughts                    ;
DROP TABLE IF EXISTS user_companions                  ;
DROP TABLE IF EXISTS user_weapon_notes                ;
DROP TABLE IF EXISTS user_weapon_stories              ;
DROP TABLE IF EXISTS user_weapon_awakens              ;
DROP TABLE IF EXISTS user_weapon_abilities            ;
DROP TABLE IF EXISTS user_weapon_skills               ;
DROP TABLE IF EXISTS user_weapons                     ;
DROP TABLE IF EXISTS user_costume_awaken_status_ups   ;
DROP TABLE IF EXISTS user_costume_active_skills       ;
DROP TABLE IF EXISTS user_costumes                    ;
DROP TABLE IF EXISTS user_character_rebirths          ;
DROP TABLE IF EXISTS user_character_board_status_ups  ;
DROP TABLE IF EXISTS user_character_board_abilities   ;
DROP TABLE IF EXISTS user_character_boards            ;
DROP TABLE IF EXISTS user_characters                  ;
DROP TABLE IF EXISTS user_gacha                       ;
DROP TABLE IF EXISTS user_shop_replaceable            ;
DROP TABLE IF EXISTS user_explore                     ;
DROP TABLE IF EXISTS user_guerrilla_free_open         ;
DROP TABLE IF EXISTS user_portal_cage                 ;
DROP TABLE IF EXISTS user_notification                ;
DROP TABLE IF EXISTS user_battle                      ;
DROP TABLE IF EXISTS user_big_hunt_state              ;
DROP TABLE IF EXISTS user_side_story_active           ;
DROP TABLE IF EXISTS user_extra_quest                 ;
DROP TABLE IF EXISTS user_event_quest                 ;
DROP TABLE IF EXISTS user_main_quest                  ;
DROP TABLE IF EXISTS user_login_bonus                 ;
DROP TABLE IF EXISTS user_login                       ;
DROP TABLE IF EXISTS user_profile                     ;
DROP TABLE IF EXISTS user_gem                         ;
DROP TABLE IF EXISTS user_status                      ;
DROP TABLE IF EXISTS user_setting                     ;
DROP TABLE IF EXISTS sessions                         ;
DROP TABLE IF EXISTS users                            ;


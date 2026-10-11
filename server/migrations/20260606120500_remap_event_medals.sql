-- +goose Up
-- The Coffin of Repose and Rhythm's Citadel events award tiered medal variants
-- (Copper/Silver/Bronze/Gold "X Medal") but their exchange shop only accepts the
-- base "X Medal", so the awarded medals are unspendable. Convert already-held
-- tier variants into the spendable base medal.
--   Coffin of Repose: 53 (Copper), 54 (Silver)            -> 22 (base)
--   Rhythm's Citadel: 63 (Bronze), 64 (Silver), 65 (Gold) -> 29 (base)

-- 53 -> 22
UPDATE user_consumable_items SET count = count + COALESCE((SELECT b.count FROM user_consumable_items b WHERE b.user_id = user_consumable_items.user_id AND b.consumable_item_id = 53), 0) WHERE consumable_item_id = 22;
UPDATE OR IGNORE user_consumable_items SET consumable_item_id = 22 WHERE consumable_item_id = 53;
DELETE FROM user_consumable_items WHERE consumable_item_id = 53;

-- 54 -> 22
UPDATE user_consumable_items SET count = count + COALESCE((SELECT b.count FROM user_consumable_items b WHERE b.user_id = user_consumable_items.user_id AND b.consumable_item_id = 54), 0) WHERE consumable_item_id = 22;
UPDATE OR IGNORE user_consumable_items SET consumable_item_id = 22 WHERE consumable_item_id = 54;
DELETE FROM user_consumable_items WHERE consumable_item_id = 54;

-- 63 -> 29
UPDATE user_consumable_items SET count = count + COALESCE((SELECT b.count FROM user_consumable_items b WHERE b.user_id = user_consumable_items.user_id AND b.consumable_item_id = 63), 0) WHERE consumable_item_id = 29;
UPDATE OR IGNORE user_consumable_items SET consumable_item_id = 29 WHERE consumable_item_id = 63;
DELETE FROM user_consumable_items WHERE consumable_item_id = 63;

-- 64 -> 29
UPDATE user_consumable_items SET count = count + COALESCE((SELECT b.count FROM user_consumable_items b WHERE b.user_id = user_consumable_items.user_id AND b.consumable_item_id = 64), 0) WHERE consumable_item_id = 29;
UPDATE OR IGNORE user_consumable_items SET consumable_item_id = 29 WHERE consumable_item_id = 64;
DELETE FROM user_consumable_items WHERE consumable_item_id = 64;

-- 65 -> 29
UPDATE user_consumable_items SET count = count + COALESCE((SELECT b.count FROM user_consumable_items b WHERE b.user_id = user_consumable_items.user_id AND b.consumable_item_id = 65), 0) WHERE consumable_item_id = 29;
UPDATE OR IGNORE user_consumable_items SET consumable_item_id = 29 WHERE consumable_item_id = 65;
DELETE FROM user_consumable_items WHERE consumable_item_id = 65;

-- +goose Down
-- Irreversible: the original tier/base split is not recoverable.
SELECT 1;

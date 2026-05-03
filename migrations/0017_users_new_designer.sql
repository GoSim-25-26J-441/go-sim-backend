-- Patterns designer: whether to show guided highlights (AMG-APD) for the user.
-- "Yes" for new accounts by default; toggled from the Patterns page Guides control.

ALTER TABLE users
ADD COLUMN IF NOT EXISTS new_designer VARCHAR(3) NOT NULL DEFAULT 'Yes';

UPDATE users
SET new_designer = 'Yes'
WHERE new_designer IS NULL OR TRIM(new_designer) = '';

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_new_designer_check;
ALTER TABLE users ADD CONSTRAINT users_new_designer_check
CHECK (new_designer IN ('Yes', 'No'));

COMMENT ON COLUMN users.new_designer IS 'Yes = show patterns guides by default; No = hide guides';

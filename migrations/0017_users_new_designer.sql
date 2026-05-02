-- Persist AMG-APD patterns "Guides" preference per user ("Yes" | "No").
-- Default "Yes" for new rows; existing users get "Yes" via ADD COLUMN ... DEFAULT.

ALTER TABLE users
ADD COLUMN IF NOT EXISTS new_designer VARCHAR(10) NOT NULL DEFAULT 'Yes'
    CHECK (new_designer IN ('Yes', 'No'));

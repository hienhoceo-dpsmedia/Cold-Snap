ALTER TABLE destination
  ADD COLUMN IF NOT EXISTS append_path boolean NOT NULL DEFAULT false;


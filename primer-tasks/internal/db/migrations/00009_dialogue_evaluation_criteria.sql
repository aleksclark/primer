-- Persist the parent-rubric criteria used by each dialogue evaluation.
-- This keeps inspect/audit evidence complete without storing model reasoning.
ALTER TABLE verification_evaluations
  ADD COLUMN IF NOT EXISTS criteria jsonb NOT NULL DEFAULT '[]'::jsonb;

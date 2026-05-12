-- Migration 004 rollback

ALTER TABLE executions DROP COLUMN IF EXISTS output_text;

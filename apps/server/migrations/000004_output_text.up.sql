-- Migration 004: add output_text to executions for cron stdout/stderr capture

ALTER TABLE executions ADD COLUMN output_text TEXT;

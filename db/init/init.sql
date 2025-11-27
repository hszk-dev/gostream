-- PostgreSQL initialization script for gostream
-- This runs on first container startup

-- Enable pg_stat_statements extension for query analysis
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Webhook Proxy Database Initialization
-- This script is executed when the PostgreSQL container is first created

-- Create additional extensions if needed
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Set default timezone
SET timezone = 'UTC';

-- Create indexes for performance (these will be created by Drizzle migrations)
-- The actual schema will be created by the application's migration system
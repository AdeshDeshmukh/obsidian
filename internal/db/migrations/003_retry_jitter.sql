-- 003_retry_jitter.sql
-- Add exponential_jitter as a valid retry strategy

ALTER TABLE retry_policies
    DROP CONSTRAINT IF EXISTS retry_policies_strategy_check;

ALTER TABLE retry_policies
    ADD CONSTRAINT retry_policies_strategy_check
    CHECK (strategy IN ('fixed', 'linear', 'exponential', 'exponential_jitter'));

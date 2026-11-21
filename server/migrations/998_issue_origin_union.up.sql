-- Numbered 998 on purpose: the runner sorts file names as strings, and this has
-- to run after 667_issue_origin_weixin_chat and 904_issue_origin_directive.
-- Each of them recreated issue_origin_type_check with a list that lacks the
-- other's values (weixin_chat from upstream; proposal, squad_request and
-- directive from the fork), so whichever ran last rejected the other's issues.
-- This one allows all of them. NOT VALID keeps it catalog-only: widening a
-- whitelist cannot fail on existing rows.
ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_origin_type_check;
ALTER TABLE issue ADD CONSTRAINT issue_origin_type_check
    CHECK (origin_type IN ('autopilot', 'quick_create', 'lark_chat', 'slack_chat', 'agent_create', 'dingtalk_chat', 'wecom_chat', 'telegram_chat', 'weixin_chat', 'proposal', 'squad_request', 'directive'))
    NOT VALID;

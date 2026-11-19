UPDATE plan SET limits = limits - 'max_workspaces'
WHERE key IN ('free', 'pro', 'business', 'payg', 'enterprise', 'unlimited');

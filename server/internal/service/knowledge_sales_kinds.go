package service

// The fields of the Sales Loop kinds (spec 6.2). A kind's fields are the values a squad filters,
// sorts and moves on a board; the Record section of a document carries the same keys as text.

var signalFamilyOptions = []string{
	"hiring", "funding", "expansion", "launch", "regulatory", "tender", "event", "pain", "site_change",
	"registry", "competitor_customer", "trade_flow", "leadership_change", "directory", "own_data",
	"inbound", "ad_activity",
}

var (
	salesGoalOptions    = []string{"direct_sale", "local_business", "channel", "tender", "export_buyer"}
	salesChannelOptions = []string{"email", "phone", "form", "chat", "visit", "introduction", "public_reply"}
	salesStageOptions   = []string{"new", "watch", "qualified", "contacted", "replied", "meeting", "proposal", "won", "lost", "disqualified"}
	salesDealStages     = []string{"discovery", "proposal", "negotiation", "won", "lost"}
	salesSourceStatus   = []string{"proposed", "active", "needs_connector", "manual_only", "degraded", "blocked", "disabled"}
	salesSourcePlatform = []string{"web", "facebook", "instagram", "zalo", "tiktok", "youtube", "linkedin", "reddit", "x", "telegram", "forum", "maps", "registry", "tender", "file", "other"}
	salesSourceAdapter  = []string{"rss", "json_api", "html_jsonld", "html_list", "web_search", "csv_drop", "kb_internal", "social_connector", "provider", "human_share", "mailbox"}
)

func selectField(key, label string, ordered bool, options ...string) KindField {
	return KindField{Key: key, Label: label, Type: KindFieldSelect, Options: options, Ordered: ordered}
}

func textField(key, label string) KindField {
	return KindField{Key: key, Label: label, Type: KindFieldText}
}
func numberField(key, label string) KindField {
	return KindField{Key: key, Label: label, Type: KindFieldNumber}
}
func dateField(key, label string) KindField {
	return KindField{Key: key, Label: label, Type: KindFieldDate}
}
func boolField(key, label string) KindField {
	return KindField{Key: key, Label: label, Type: KindFieldBool}
}
func urlField(key, label string) KindField {
	return KindField{Key: key, Label: label, Type: KindFieldURL}
}

// salesKindFields maps each Sales Loop kind to its fields. A kind not listed has none.
var salesKindFields = map[string][]KindField{
	"sales_lane": {
		selectField("goal", "Goal", false, salesGoalOptions...),
		selectField("status", "Status", false, "draft", "active", "paused"),
		textField("regions", "Regions"),
		textField("offer", "Offer"),
		textField("families", "Families"),
		numberField("weekly_cap", "Weekly cap"),
		numberField("min_score", "Minimum score"),
	},
	"sales_source": {
		selectField("platform", "Platform", false, salesSourcePlatform...),
		selectField("family", "Family", false, signalFamilyOptions...),
		selectField("adapter", "Adapter", false, salesSourceAdapter...),
		numberField("rung", "Rung"),
		urlField("url", "Address"),
		textField("regions", "Regions"),
		textField("sectors", "Sectors"),
		selectField("auth", "Auth", false, "none", "key", "oauth", "provider_token"),
		selectField("terms", "Terms", false, "ok", "attribution", "restricted", "unknown"),
		selectField("status", "Status", false, salesSourceStatus...),
		selectField("added_by", "Added by", false, "starter", "user", "discovery", "share"),
		dateField("last_run", "Last run"),
		numberField("last_http", "Last HTTP status"),
		numberField("hits_7d", "Items, 7 days"),
		numberField("hits_30d", "Items, 30 days"),
		numberField("qualified_30d", "Qualified, 30 days"),
		numberField("cost_30d", "Cost, 30 days (USD)"),
		numberField("recipe_version", "Recipe version"),
	},
	"sales_account": {
		textField("entity_key", "Entity key"),
		textField("lane", "Lane"),
		textField("country", "Country"),
		textField("province", "Province"),
		textField("sector", "Sector"),
		selectField("size_band", "Size", false, "micro", "small", "medium", "large", "unknown"),
		selectField("signal_family", "Signal", false, signalFamilyOptions...),
		urlField("signal_url", "Signal link"),
		dateField("signal_date", "Signal date"),
		numberField("fit_score", "Score"),
		selectField("stage", "Stage", true, salesStageOptions...),
		selectField("stage_set_by", "Stage set by", false, "agent", "person"),
		boolField("do_not_contact", "Do not contact"),
		selectField("contact_channel", "Channel", false, salesChannelOptions...),
		textField("contact_value", "Contact"),
		urlField("contact_source", "Contact source"),
		textField("owner", "Owner"),
		dateField("next_action_date", "Next action"),
		dateField("last_touch", "Last touch"),
	},
	"sales_opportunity": {
		textField("account", "Account"),
		textField("lane", "Lane"),
		selectField("stage", "Stage", true, salesDealStages...),
		numberField("value", "Value"),
		textField("currency", "Currency"),
		textField("timeline", "Timeline"),
		selectField("budget_signal", "Budget", false, "none", "hinted", "confirmed"),
		textField("close_reason", "Close reason"),
		dateField("next_action_date", "Next action"),
	},
	"sales_outreach": {
		textField("account", "Account"),
		textField("opportunity", "Opportunity"),
		textField("lane", "Lane"),
		selectField("channel", "Channel", false, salesChannelOptions...),
		selectField("state", "State", false, "draft", "checked", "approved", "sent", "replied", "rejected"),
		selectField("verdict", "Verdict", false, "pass", "concerns", "fail"),
		selectField("compliance", "Compliance", false, "ok", "missing"),
		dateField("sent_date", "Sent"),
	},
	"sales_lesson": {
		textField("lane", "Lane"),
		numberField("evidence_count", "Evidence"),
		selectField("family", "Family", false, signalFamilyOptions...),
		selectField("effect", "Effect", false, "raise_weight", "lower_weight", "new_rule", "template_change"),
		selectField("confidence", "Confidence", false, "low", "medium", "high"),
	},
	"sales_share": {
		selectField("platform", "Platform", false, salesSourcePlatform...),
		urlField("url", "Address"),
		selectField("state", "State", false, "new", "processed", "rejected", "expired"),
		textField("shared_by", "Shared by"),
		dateField("shared_at", "Shared"),
		selectField("intent", "Intent", false, "seeking", "complaint", "announcement", "hiring", "irrelevant"),
		textField("account", "Account"),
		dateField("expires_at", "Expires"),
	},
	"sales_icp":       {textField("lane", "Lane"), dateField("valid_until", "Valid until")},
	"sales_offer":     {textField("lane", "Lane"), dateField("valid_until", "Valid until")},
	"sales_proof":     {textField("lane", "Lane")},
	"sales_voice":     {textField("language", "Language")},
	"sales_templates": {textField("lane", "Lane"), selectField("channel", "Channel", false, salesChannelOptions...), textField("goal", "Goal")},
	"sales_report": {
		dateField("window_start", "Window start"),
		dateField("window_end", "Window end"),
		textField("lanes", "Lanes"),
	},
}

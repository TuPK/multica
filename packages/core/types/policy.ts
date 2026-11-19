export interface PolicyFloor {
  allowed_invite_domains: string[];
  share_links_enabled: boolean;
  require_sso: boolean;
  require_mfa: boolean;
  session_max_age_hours: number;
}

import { skeletonRecipe, type SalesFamily, type SalesMethod, type SalesRecipe } from "./sales-recipe";

/**
 * The starter catalogue of a Sales Loop squad: the sources it starts with, so a lane runs with no
 * configuration. Each entry becomes one `sales_source` document in the knowledge base base "sales",
 * with the idempotency key `sales:source:<id>`, so running the quickstart again creates nothing twice.
 *
 * Status is decided by evidence, not by hope:
 * - `active`: the recipe was checked on a saved response (testdata in server/internal/salescollect) and
 *   needs no connection, key or budget.
 * - `needs_connector`: a legal route exists, and the person's connection, key or budget opens it. The
 *   `unlock` sentence says which, what it costs (estimated), and what it opens.
 * - `manual_only`: the only legal route is the person's share.
 * - `proposed`: not yet verified. The Scout checks it on its first run (route E), and it becomes `active`
 *   or stops in a stated state. Nothing here is treated as working until that check passes.
 */

export type SalesSourceStatus = "active" | "needs_connector" | "manual_only" | "proposed";
export type SalesPlatform =
  | "web"
  | "facebook"
  | "instagram"
  | "zalo"
  | "tiktok"
  | "youtube"
  | "linkedin"
  | "reddit"
  | "x"
  | "telegram"
  | "forum"
  | "maps"
  | "registry"
  | "tender"
  | "file"
  | "other";
export type SalesGoal = "direct_sale" | "local_business" | "channel" | "tender" | "export_buyer";
export type SalesTerms = "ok" | "attribution" | "restricted" | "unknown";

export interface SalesStarterSource {
  /** Stable catalogue id. The document's idempotency key is `sales:source:<id>`. */
  id: string;
  title: string;
  platform: SalesPlatform;
  /** The family the source feeds. Null for a social source: the family comes from each post's content. */
  family: SalesFamily | null;
  method: SalesMethod;
  rung: number;
  url: string;
  goals: readonly SalesGoal[];
  regions: string;
  terms: SalesTerms;
  status: SalesSourceStatus;
  /** For needs_connector and manual_only: the sentence that says what unlocks it. */
  unlock?: string;
  /** What the source gives a lane, in one sentence. */
  gives: string;
  recipe: SalesRecipe;
}

const ALL_GOALS: readonly SalesGoal[] = ["direct_sale", "local_business", "channel", "tender", "export_buyer"];

/** A verified recipe: the field paths were read from a saved response. */
function verified(
  base: SalesRecipe,
  coverage: number,
  count: number,
): SalesRecipe {
  return { ...base, verified_at: "2026-11-10", sample_count: count, field_coverage: coverage };
}

const feed = (url: string, family: SalesFamily, keywords: string[] = []) => ({
  ...skeletonRecipe({
    rung: 2,
    method: "rss",
    url,
    family,
    fields: { entity: "title", url: "link", observed_at: "pubDate", text: "description" },
  }),
  filter: { keywords_any: keywords, keywords_none: [], language: "", since_days: 30 },
});

export const SALES_STARTER_SOURCES: readonly SalesStarterSource[] = [
  // News and company signals (every industry)
  {
    id: "news-gnews-expansion",
    title: "Google News: expansion in Vietnamese",
    platform: "web",
    family: "expansion",
    method: "rss",
    rung: 2,
    url: "https://news.google.com/rss/search?q=khai+tr%C6%B0%C6%A1ng+OR+m%E1%BB%9F+chi+nh%C3%A1nh&hl=vi&gl=VN&ceid=VN:vi",
    goals: ["direct_sale", "local_business", "channel"],
    regions: "VN",
    terms: "attribution",
    status: "active",
    gives: "Local openings and new branches, from Vietnamese news, one row per article with its publisher.",
    recipe: verified(feed("https://news.google.com/rss/search?q=khai+tr%C6%B0%C6%A1ng+OR+m%E1%BB%9F+chi+nh%C3%A1nh&hl=vi&gl=VN&ceid=VN:vi", "expansion"), 1, 5),
  },
  {
    id: "news-gnews-funding",
    title: "Google News: funding in Vietnamese",
    platform: "web",
    family: "funding",
    method: "rss",
    rung: 2,
    url: "https://news.google.com/rss/search?q=g%E1%BB%8Di+v%E1%BB%91n+OR+hu%E1%BB%87+%C4%91%E1%BB%99ng+v%E1%BB%91n&hl=vi&gl=VN&ceid=VN:vi",
    goals: ["direct_sale", "channel"],
    regions: "VN",
    terms: "attribution",
    status: "proposed",
    gives: "Funding rounds and loans announced in Vietnamese news.",
    recipe: feed("https://news.google.com/rss/search?q=g%E1%BB%8Di+v%E1%BB%91n&hl=vi&gl=VN&ceid=VN:vi", "funding"),
  },
  {
    id: "news-gnews-regulatory",
    title: "Google News: new regulation in Vietnamese",
    platform: "web",
    family: "regulatory",
    method: "rss",
    rung: 2,
    url: "https://news.google.com/rss/search?q=ngh%E1%BB%8B+%C4%91%E1%BB%8Bnh+m%E1%BB%9Bi&hl=vi&gl=VN&ceid=VN:vi",
    goals: ["direct_sale", "tender", "local_business"],
    regions: "VN",
    terms: "attribution",
    status: "proposed",
    gives: "New decrees and standards that create a deadline for a business.",
    recipe: feed("https://news.google.com/rss/search?q=ngh%E1%BB%8B+%C4%91%E1%BB%8Bnh+m%E1%BB%9Bi&hl=vi&gl=VN&ceid=VN:vi", "regulatory"),
  },
  {
    id: "news-vnexpress-business",
    title: "VnExpress Business (RSS)",
    platform: "web",
    family: "expansion",
    method: "rss",
    rung: 2,
    url: "https://vnexpress.net/rss/kinh-doanh.rss",
    goals: ["direct_sale", "local_business"],
    regions: "VN",
    terms: "attribution",
    status: "proposed",
    gives: "Business news from a general daily; filter by the lane's keywords.",
    recipe: feed("https://vnexpress.net/rss/kinh-doanh.rss", "expansion", ["mở rộng", "khai trương", "chi nhánh"]),
  },
  {
    id: "news-techinasia",
    title: "Tech in Asia (RSS)",
    platform: "web",
    family: "funding",
    method: "rss",
    rung: 2,
    url: "https://www.techinasia.com/feed",
    goals: ["direct_sale", "channel"],
    regions: "Southeast Asia",
    terms: "attribution",
    status: "active",
    gives: "Startup funding and launches in Southeast Asia, one row per article.",
    recipe: verified(feed("https://www.techinasia.com/feed", "funding"), 1, 5),
  },
  {
    id: "launch-producthunt",
    title: "Product Hunt launches (Atom)",
    platform: "web",
    family: "launch",
    method: "rss",
    rung: 2,
    url: "https://www.producthunt.com/feed",
    goals: ["direct_sale"],
    regions: "global",
    terms: "attribution",
    status: "active",
    gives: "New products launched, with their makers' pages: useful for software lanes.",
    recipe: verified(
      {
        ...skeletonRecipe({
          rung: 2,
          method: "rss",
          url: "https://www.producthunt.com/feed",
          family: "launch",
          fields: { entity: "title", url: "link", observed_at: "published", text: "summary" },
        }),
      },
      1,
      5,
    ),
  },
  {
    id: "trends-google-vn",
    title: "Google Trends, Vietnam (RSS)",
    platform: "web",
    family: "pain",
    method: "rss",
    rung: 2,
    url: "https://trends.google.com/trending/rss?geo=VN",
    goals: ["direct_sale", "local_business"],
    regions: "VN",
    terms: "unknown",
    status: "proposed",
    gives: "Topics rising now. Not organisations: use them to suggest queries for a lane, never as an account.",
    recipe: feed("https://trends.google.com/trending/rss?geo=VN", "pain"),
  },

  // Hiring: demand by function, for any function
  {
    id: "jobs-remotive",
    title: "Remotive remote jobs (API)",
    platform: "web",
    family: "hiring",
    method: "json_api",
    rung: 3,
    url: "https://remotive.com/api/remote-jobs",
    goals: ["direct_sale", "channel"],
    regions: "global, remote",
    terms: "attribution",
    status: "active",
    gives: "Remote job postings by function. A hiring for a function is a signal for an offer that serves that function.",
    recipe: verified(
      {
        ...skeletonRecipe({
          rung: 3,
          method: "json_api",
          url: "https://remotive.com/api/remote-jobs",
          family: "hiring",
          items: "jobs",
          fields: { entity: "company_name", url: "url", observed_at: "publication_date", text: "title" },
        }),
      },
      1,
      5,
    ),
  },
  {
    id: "jobs-remoteok",
    title: "RemoteOK jobs (API)",
    platform: "web",
    family: "hiring",
    method: "json_api",
    rung: 3,
    url: "https://remoteok.com/api",
    goals: ["direct_sale", "channel"],
    regions: "global, remote",
    terms: "attribution",
    status: "active",
    gives: "Remote job postings; the terms require the source and a link on every reuse.",
    recipe: verified(
      {
        ...skeletonRecipe({
          rung: 3,
          method: "json_api",
          url: "https://remoteok.com/api",
          family: "hiring",
          items: "",
          fields: { entity: "company", url: "url", observed_at: "date", text: "position" },
        }),
      },
      1,
      5,
    ),
  },
  {
    id: "jobs-arbeitnow",
    title: "Arbeitnow job board (API)",
    platform: "web",
    family: "hiring",
    method: "json_api",
    rung: 3,
    url: "https://www.arbeitnow.com/api/job-board-api",
    goals: ["channel", "export_buyer"],
    regions: "Europe",
    terms: "ok",
    status: "active",
    gives: "European job postings, mostly Germany, the UK, France and Switzerland. Use for European lanes only.",
    recipe: verified(
      {
        ...skeletonRecipe({
          rung: 3,
          method: "json_api",
          url: "https://www.arbeitnow.com/api/job-board-api",
          family: "hiring",
          items: "data",
          fields: { entity: "company_name", url: "url", observed_at: "created_at", text: "title" },
        }),
      },
      1,
      5,
    ),
  },
  {
    id: "jobs-weworkremotely",
    title: "We Work Remotely (RSS)",
    platform: "web",
    family: "hiring",
    method: "rss",
    rung: 2,
    url: "https://weworkremotely.com/remote-jobs.rss",
    goals: ["direct_sale"],
    regions: "global, remote",
    terms: "attribution",
    status: "proposed",
    gives: "Remote job postings from a job board, by category.",
    recipe: feed("https://weworkremotely.com/remote-jobs.rss", "hiring"),
  },
  {
    id: "jobs-itviec",
    title: "ITviec, IT jobs in Vietnam",
    platform: "web",
    family: "hiring",
    method: "html_jsonld",
    rung: 4,
    url: "https://itviec.com/it-jobs",
    goals: ["direct_sale"],
    regions: "VN",
    terms: "unknown",
    status: "proposed",
    gives: "IT job postings in Vietnam. Only for lanes that sell to technology teams.",
    recipe: skeletonRecipe({ rung: 4, method: "html_jsonld", url: "https://itviec.com/it-jobs", family: "hiring", items: "ListItem", fields: { url: "url" } }),
  },
  {
    id: "jobs-careerviet",
    title: "CareerViet, jobs in Vietnam",
    platform: "web",
    family: "hiring",
    method: "html_list",
    rung: 4,
    url: "https://careerviet.vn/viec-lam/cntt-phan-mem-c1-vi.html",
    goals: ["direct_sale", "local_business", "channel"],
    regions: "VN",
    terms: "ok",
    status: "proposed",
    gives: "Job postings in Vietnam across functions; the robots file allows reading.",
    recipe: skeletonRecipe({ rung: 4, method: "html_list", url: "https://careerviet.vn/viec-lam/cntt-phan-mem-c1-vi.html", family: "hiring" }),
  },

  // Tenders and procurement
  {
    id: "tender-worldbank",
    title: "World Bank procurement notices (API)",
    platform: "tender",
    family: "tender",
    method: "json_api",
    rung: 3,
    url: "https://search.worldbank.org/api/v2/procnotices?format=json&rows=50",
    goals: ["tender", "export_buyer"],
    regions: "global",
    terms: "ok",
    status: "proposed",
    gives: "Procurement notices from World Bank projects, with the deadline and the country.",
    recipe: skeletonRecipe({ rung: 3, method: "json_api", url: "https://search.worldbank.org/api/v2/procnotices?format=json&rows=50", family: "tender", items: "procnotices", fields: { entity: "project_name", url: "url", observed_at: "noticedate", text: "notice_type" } }),
  },
  {
    id: "tender-uk-contracts",
    title: "UK Contracts Finder (OCDS)",
    platform: "tender",
    family: "tender",
    method: "json_api",
    rung: 3,
    url: "https://www.contractsfinder.service.gov.uk/Search/Results",
    goals: ["tender", "export_buyer"],
    regions: "UK",
    terms: "ok",
    status: "proposed",
    gives: "UK public contract notices in the open contracting format.",
    recipe: skeletonRecipe({ rung: 3, method: "json_api", url: "https://www.contractsfinder.service.gov.uk/Search/Results", family: "tender" }),
  },
  {
    id: "tender-muasamcong",
    title: "Vietnam public procurement portal",
    platform: "tender",
    family: "tender",
    method: "html_list",
    rung: 4,
    url: "https://muasamcong.mof.gov.vn",
    goals: ["tender"],
    regions: "VN",
    terms: "unknown",
    status: "proposed",
    gives: "Bid invitations on the national procurement portal. The extraction is not yet checked.",
    recipe: skeletonRecipe({ rung: 4, method: "html_list", url: "https://muasamcong.mof.gov.vn", family: "tender" }),
  },
  {
    id: "tender-ted-eu",
    title: "TED, EU tenders",
    platform: "tender",
    family: "tender",
    method: "json_api",
    rung: 3,
    url: "https://ted.europa.eu",
    goals: ["tender", "export_buyer"],
    regions: "EU",
    terms: "unknown",
    status: "proposed",
    gives: "EU tender notices. The public site answered 429 on the first try; an API key may be needed.",
    recipe: skeletonRecipe({ rung: 3, method: "json_api", url: "https://ted.europa.eu", family: "tender" }),
  },

  // Registry and enrichment
  {
    id: "registry-dkkd",
    title: "National business registration lookup",
    platform: "registry",
    family: "registry",
    method: "html_list",
    rung: 4,
    url: "https://dangkykinhdoanh.gov.vn",
    goals: ALL_GOALS,
    regions: "VN",
    terms: "unknown",
    status: "proposed",
    gives: "Verifies an organisation's legal name, status and address. Used to check an account, never to discover accounts in bulk.",
    recipe: skeletonRecipe({ rung: 4, method: "html_list", url: "https://dangkykinhdoanh.gov.vn", family: "registry" }),
  },

  // Community and forums (open formats)
  {
    id: "forum-voz",
    title: "Vietnamese forum (XenForo RSS, example: voz.vn)",
    platform: "forum",
    family: "pain",
    method: "rss",
    rung: 2,
    url: "https://voz.vn/forums/-/index.rss",
    goals: ["direct_sale", "local_business"],
    regions: "VN",
    terms: "unknown",
    status: "proposed",
    gives: "Questions and complaints in a public forum. Replace the forum with one from the lane's industry.",
    recipe: feed("https://voz.vn/forums/-/index.rss", "pain"),
  },
  {
    id: "community-reddit",
    title: "Reddit subreddit (RSS): enter the subreddit",
    platform: "reddit",
    family: "pain",
    method: "rss",
    rung: 2,
    url: "https://www.reddit.com/r/<subreddit>/new/.rss",
    goals: ["direct_sale"],
    regions: "global",
    terms: "attribution",
    status: "proposed",
    gives: "Posts in one subreddit. The feed has no score or comment count. Enter the subreddit of the lane's industry.",
    recipe: feed("https://www.reddit.com/r/<subreddit>/new/.rss", "pain"),
  },
  {
    id: "community-telegram",
    title: "Telegram public channel (t.me/s): enter the channel",
    platform: "telegram",
    family: "pain",
    method: "html_list",
    rung: 2,
    url: "https://t.me/s/<channel>",
    goals: ["direct_sale", "local_business", "channel"],
    regions: "global",
    terms: "unknown",
    status: "proposed",
    gives: "The latest posts of a public channel. Private groups are not readable and are shared by a person instead.",
    recipe: skeletonRecipe({ rung: 2, method: "html_list", url: "https://t.me/s/<channel>", family: "pain", items: ".tgme_widget_message", fields: { entity: "", url: ".tgme_widget_message_date@href", observed_at: "time@datetime", text: ".tgme_widget_message_text" } }),
  },

  // Own data and the squad's own knowledge
  {
    id: "own-customers-csv",
    title: "Your customer list (CSV you drop in)",
    platform: "file",
    family: "own_data",
    method: "csv_drop",
    rung: 7,
    url: "",
    goals: ALL_GOALS,
    regions: "any",
    terms: "ok",
    status: "manual_only",
    unlock: "Drop a CSV of past customers or leads in Source Studio's share box, with the columns name, website and province.",
    gives: "Your own past customers and leads, to find look-alikes and to avoid contacting them twice.",
    recipe: skeletonRecipe({ rung: 7, method: "csv_drop", url: "", family: "own_data" }),
  },
  {
    id: "kb-competitor-customers",
    title: "Competitor case studies (from the knowledge base)",
    platform: "other",
    family: "competitor_customer",
    method: "kb_internal",
    rung: 1,
    url: "kb:competitor_profile",
    goals: ["direct_sale", "channel"],
    regions: "any",
    terms: "ok",
    status: "active",
    gives: "Customers named in competitor profiles that other squads wrote into the knowledge base.",
    recipe: skeletonRecipe({ rung: 1, method: "kb_internal", url: "kb:competitor_profile", family: "competitor_customer" }),
  },

  // Social networks (one row per network, rung by rung; see spec section 4.10)
  {
    id: "social-facebook-own-page",
    title: "Facebook: your Page (comments, messages, lead forms)",
    platform: "facebook",
    family: null,
    method: "social_connector",
    rung: 1,
    url: "https://www.facebook.com/<your-page>",
    goals: ALL_GOALS,
    regions: "any",
    terms: "ok",
    status: "needs_connector",
    unlock: "Connect Facebook for your Page in the Connections panel (free, OAuth). Opens the comments, messages and lead forms of that Page only.",
    gives: "Inbound interest on your own Page: comments, messages and lead-form submissions.",
    recipe: skeletonRecipe({ rung: 1, method: "social_connector", url: "https://www.facebook.com/<your-page>", family: "inbound" }),
  },
  {
    id: "social-facebook-public-pages",
    title: "Facebook: public Pages of named organisations (provider)",
    platform: "facebook",
    family: null,
    method: "provider",
    rung: 6,
    url: "https://www.facebook.com/<organisation-page>",
    goals: ALL_GOALS,
    regions: "any",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Connect Apify and set a monthly cap (about 5 to 12 USD per 1,000 pages). Opens the public posts and follower counts of the Pages you name. Owner or admin confirms the first run.",
    gives: "What public Pages of named organisations post: launches, expansions, complaints in their comments.",
    recipe: { ...skeletonRecipe({ rung: 6, method: "provider", url: "https://www.facebook.com/<organisation-page>", family: null }), provider: { toolkit: "APIFY", actor: "to confirm (spec section 15, item 9)", input: {}, cost_cap_usd: 0 } },
  },
  {
    id: "social-facebook-public-groups",
    title: "Facebook: public groups (provider)",
    platform: "facebook",
    family: null,
    method: "provider",
    rung: 6,
    url: "https://www.facebook.com/groups/<public-group>",
    goals: ["direct_sale", "local_business"],
    regions: "any",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Connect Apify and set a monthly cap. Opens posts of public groups only; private groups are shared by a person (manual share).",
    gives: "Questions people ask in public groups: the posts that look for a solution.",
    recipe: { ...skeletonRecipe({ rung: 6, method: "provider", url: "https://www.facebook.com/groups/<public-group>", family: "pain" }), provider: { toolkit: "APIFY", actor: "to confirm (spec section 15, item 9)", input: {}, cost_cap_usd: 0 } },
  },
  {
    id: "social-facebook-ad-library",
    title: "Facebook Ad Library (provider, or the person's share)",
    platform: "facebook",
    family: "ad_activity",
    method: "provider",
    rung: 6,
    url: "https://www.facebook.com/ads/library/",
    goals: ["direct_sale", "channel"],
    regions: "VN",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Connect Apify and set a monthly cap (about 0.3 to 10 USD per 1,000 ads). Opens the active ads of the companies you name. Without it, share an ad you see.",
    gives: "Which companies run ads right now, and what they promote.",
    recipe: { ...skeletonRecipe({ rung: 6, method: "provider", url: "https://www.facebook.com/ads/library/", family: "ad_activity" }), provider: { toolkit: "APIFY", actor: "to confirm (spec section 15, item 9)", input: {}, cost_cap_usd: 0 }, fallbacks: [{ rung: 7 }] },
  },
  {
    id: "social-facebook-marketplace",
    title: "Facebook Marketplace and local reviews (provider)",
    platform: "facebook",
    family: "directory",
    method: "provider",
    rung: 6,
    url: "https://www.facebook.com/marketplace/",
    goals: ["local_business"],
    regions: "VN",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Connect Apify and set a monthly cap. Opens local sellers and business reviews in the area you name.",
    gives: "Local businesses and their public reviews.",
    recipe: { ...skeletonRecipe({ rung: 6, method: "provider", url: "https://www.facebook.com/marketplace/", family: "directory" }), provider: { toolkit: "APIFY", actor: "to confirm (spec section 15, item 9)", input: {}, cost_cap_usd: 0 } },
  },
  {
    id: "social-instagram-own",
    title: "Instagram: your business account",
    platform: "instagram",
    family: null,
    method: "social_connector",
    rung: 1,
    url: "https://www.instagram.com/<your-account>",
    goals: ALL_GOALS,
    regions: "any",
    terms: "ok",
    status: "needs_connector",
    unlock: "Connect Instagram for your business account (free, OAuth). Opens the comments and direct messages of that account.",
    gives: "Comments and messages on your own business account.",
    recipe: skeletonRecipe({ rung: 1, method: "social_connector", url: "https://www.instagram.com/<your-account>", family: "inbound" }),
  },
  {
    id: "social-instagram-public",
    title: "Instagram: public profiles (provider)",
    platform: "instagram",
    family: null,
    method: "provider",
    rung: 6,
    url: "https://www.instagram.com/<organisation>",
    goals: ["direct_sale", "local_business"],
    regions: "any",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Connect Apify and set a monthly cap. Opens public posts of the profiles you name. The page without login shows only a shell.",
    gives: "Public posts of named organisations.",
    recipe: { ...skeletonRecipe({ rung: 6, method: "provider", url: "https://www.instagram.com/<organisation>", family: null }), provider: { toolkit: "APIFY", actor: "to confirm (spec section 15, item 9)", input: {}, cost_cap_usd: 0 } },
  },
  {
    id: "social-zalo-oa",
    title: "Zalo Official Account: your OA",
    platform: "zalo",
    family: null,
    method: "social_connector",
    rung: 1,
    url: "https://oa.zalo.me",
    goals: ALL_GOALS,
    regions: "VN",
    terms: "ok",
    status: "needs_connector",
    unlock: "Create a Zalo app and grant it to your OA (some features need a verified OA or a paid package). Opens messages, followers and their labels.",
    gives: "Messages and followers of your own Zalo OA.",
    recipe: skeletonRecipe({ rung: 1, method: "social_connector", url: "https://oa.zalo.me", family: "inbound" }),
  },
  {
    id: "social-tiktok-business",
    title: "TikTok Business: your account (lead forms, mentions)",
    platform: "tiktok",
    family: null,
    method: "social_connector",
    rung: 1,
    url: "https://www.tiktok.com/business",
    goals: ALL_GOALS,
    regions: "any",
    terms: "ok",
    status: "needs_connector",
    unlock: "Connect your TikTok Business account (free, OAuth). Opens lead-form submissions and mentions of your account.",
    gives: "Lead-form submissions and mentions of your account.",
    recipe: skeletonRecipe({ rung: 1, method: "social_connector", url: "https://www.tiktok.com/business", family: "inbound" }),
  },
  {
    id: "social-tiktok-public",
    title: "TikTok: public videos and hashtags (provider)",
    platform: "tiktok",
    family: null,
    method: "provider",
    rung: 6,
    url: "https://www.tiktok.com/tag/<hashtag>",
    goals: ["direct_sale", "local_business"],
    regions: "any",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Connect Apify and set a monthly cap. Opens public videos of a hashtag or an account. The research API is for non-commercial researchers only.",
    gives: "Public videos and comments about a topic in the lane.",
    recipe: { ...skeletonRecipe({ rung: 6, method: "provider", url: "https://www.tiktok.com/tag/<hashtag>", family: "pain" }), provider: { toolkit: "APIFY", actor: "to confirm (spec section 15, item 9)", input: {}, cost_cap_usd: 0 } },
  },
  {
    id: "social-youtube-channels",
    title: "YouTube: channel comments and new videos",
    platform: "youtube",
    family: null,
    method: "json_api",
    rung: 3,
    url: "https://www.googleapis.com/youtube/v3/commentThreads",
    goals: ["direct_sale", "local_business"],
    regions: "any",
    terms: "ok",
    status: "needs_connector",
    unlock: "Connect YouTube (Composio) or add a Google API key. Opens comments and videos of the channels you name, within the free quota of 10,000 units a day.",
    gives: "Comments and videos on channels of named organisations: questions and complaints.",
    recipe: skeletonRecipe({ rung: 3, method: "json_api", url: "https://www.googleapis.com/youtube/v3/commentThreads", family: "pain" }),
  },
  {
    id: "social-linkedin-own",
    title: "LinkedIn: your account and company page",
    platform: "linkedin",
    family: null,
    method: "social_connector",
    rung: 1,
    url: "https://www.linkedin.com/company/<your-company>",
    goals: ALL_GOALS,
    regions: "any",
    terms: "ok",
    status: "needs_connector",
    unlock: "Connect LinkedIn (Composio) for your own account. Opens your messages and your company page's posts. Scraping LinkedIn pages is not used.",
    gives: "Your own LinkedIn messages and your company page.",
    recipe: skeletonRecipe({ rung: 1, method: "social_connector", url: "https://www.linkedin.com/company/<your-company>", family: "inbound" }),
  },
  {
    id: "social-linkedin-exports",
    title: "LinkedIn: lists you export (CSV) or posts you share",
    platform: "linkedin",
    family: null,
    method: "csv_drop",
    rung: 7,
    url: "",
    goals: ["direct_sale", "channel"],
    regions: "any",
    terms: "restricted",
    status: "manual_only",
    unlock: "Export a list from Sales Navigator you are allowed to use, and drop the CSV; or share a post or a company page link you read.",
    gives: "Companies and decision-makers in the lane's segment, from the person's own export.",
    recipe: skeletonRecipe({ rung: 7, method: "csv_drop", url: "", family: null }),
  },
  {
    id: "social-x-search",
    title: "X (Twitter): posts by keyword (paid API)",
    platform: "x",
    family: null,
    method: "provider",
    rung: 3,
    url: "https://api.x.com/2/tweets/search/recent",
    goals: ["direct_sale"],
    regions: "any",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Connect an X API app (pay per read, about 0.005 USD a post). Off by default; opens posts that match the keywords you name.",
    gives: "Public posts that match a keyword.",
    recipe: { ...skeletonRecipe({ rung: 3, method: "provider", url: "https://api.x.com/2/tweets/search/recent", family: "pain" }), provider: { toolkit: "TWITTER", actor: "search", input: {}, cost_cap_usd: 0 } },
  },
  {
    id: "social-google-maps-reviews",
    title: "Google Maps: local businesses and reviews",
    platform: "maps",
    family: "directory",
    method: "provider",
    rung: 6,
    url: "https://www.google.com/maps",
    goals: ["local_business"],
    regions: "any",
    terms: "restricted",
    status: "needs_connector",
    unlock: "Add a Places API key (billed per request) or connect Apify and set a cap. Opens local businesses and their reviews in the area you name.",
    gives: "Local businesses, their categories, and low-rated reviews that show a pain.",
    recipe: { ...skeletonRecipe({ rung: 6, method: "provider", url: "https://www.google.com/maps", family: "directory" }), provider: { toolkit: "APIFY", actor: "to confirm (spec section 15, item 9)", input: {}, cost_cap_usd: 0 } },
  },
];

/** The document title, body and write options of one starter source, as the quickstart creates it. */
export function starterSourceDocument(source: SalesStarterSource) {
  const record = [
    `platform: ${source.platform}`,
    `family: ${source.family ?? ""}`,
    `adapter: ${source.method}`,
    `rung: ${source.rung}`,
    `url: ${source.url}`,
    `regions: ${source.regions}`,
    `terms: ${source.terms}`,
    `status: ${source.status}`,
    `added_by: starter`,
    `goals: ${source.goals.join(", ")}`,
    `recipe_version: 1`,
    `last_run:`,
    `hits_7d: 0`,
    `qualified_30d: 0`,
    `cost_30d: 0`,
  ];
  const unlock = source.unlock ? `\n${source.unlock}\n` : "";
  const content = `# ${source.title}

## Record

${record.join("\n")}

## What it gives

${source.gives}
${unlock}
## How to read it

Not probed yet. The Scout checks the recipe on its first run and writes the ten signals it finds here.

## Recipe

\`\`\`json
${JSON.stringify(source.recipe, null, 2)}
\`\`\`

## Terms and limits

Terms: ${source.terms}. ${source.terms === "attribution" ? "Keep the source name and link on every use." : source.terms === "restricted" ? "Public content only, no login; the owner or admin confirms before a paid run." : ""}

## Probe log

${source.status === "active" ? "Checked against a saved response in the test fixtures." : "No attempt yet."}
`;
  return {
    kb: "sales",
    kind: "sales_source",
    title: source.title,
    content,
    idempotency_key: `sales:source:${source.id}`,
    publish: true,
    tags: ["starter", source.platform],
  };
}

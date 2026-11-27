package settings

// declaredConnections is the media server, requester, Sonarr and Radarr and Tunarr connections,
// the public addresses, and notifications.
func declaredConnections() []Setting {
	return []Setting{
		// --- Connections: media server (§15, Phase 5) ---
		{
			Key: "library.flavor", EnvVar: "LIBRARY_FLAVOR", Group: GroupMediaServer,
			Kind: KindEnum, Enum: []EnumOption{opt("emby", "Emby"), opt("jellyfin", "Jellyfin")}, Default: "",
			Doc:    "Which media server you run, Emby or Jellyfin.",
			Detail: "They sign in differently, so Loomarr needs to know which one you run.",
		},
		{
			Key: "library.url", EnvVar: "LIBRARY_URL", Group: GroupMediaServer,
			Kind: KindURL, Default: "",
			Doc:    "The address of your media server.",
			Detail: "For example http://emby:8096.",
		},
		{
			Key: "library.token", EnvVar: "LIBRARY_TOKEN", Group: GroupMediaServer,
			Kind: KindSecret, Default: "",
			Doc:    "An API key from your media server.",
			Detail: "It lets Loomarr read your library and set up the TV guide.",
		},
		{
			// Translates the media server's folder paths to where Loomarr reads the same mount, so
			// folder discovery can offer them as library folders (#2151). Playout never uses it: it
			// plays the files Loomarr's own scan found (step 16).
			Key: "library.path_map", EnvVar: "LIBRARY_PATH_MAP", Group: GroupMediaServer,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Where your media server's folders are on this machine, so Loomarr can find your media folders.",
			Detail: "Your media server names each folder by its OWN path (e.g. /data/tv); if the same folder is mounted somewhere else on the machine running Loomarr (e.g. /mnt/media/tv), map one to the other as \"/data=>/mnt/media\". Multiple rules are separated by commas or newlines. Leave empty when the paths already match, or add your folders by hand in Settings → Library.",
		},
		{
			// The scanner's kill switch (#2151): passes, folder watches and header reads stop.
			Key: "library.scanner.enabled", Label: "Scan media folders", EnvVar: "LIBRARY_SCANNER_ENABLED",
			Group: GroupMediaServer, Kind: KindBool, Default: true, Advanced: true,
			Doc:    "Whether Loomarr reads your media folders itself to learn what each file is.",
			Detail: "Turn this off to stop it reading them; what it already learned is kept.",
		},
		// --- Connections: requester (§15, Phase 6) ---
		// How Loomarr acquires missing titles: through Seerr (default), or Sonarr + Radarr
		// directly. The provider gates which fields show (ShowWhen), mirroring llm.provider.
		{
			Key: "requester.provider", EnvVar: "REQUESTER_PROVIDER", Group: GroupRequester,
			Kind: KindEnum, Enum: []EnumOption{opt("seerr", "Seerr / Jellyseerr"), opt("arr", "Sonarr + Radarr (direct)")}, Default: "seerr",
			Doc: "How Loomarr downloads missing titles: through Seerr, or Sonarr and Radarr directly.",
		},
		{
			Key: "seerr.url", EnvVar: "SEERR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "", Required: FeatureAcquisition,
			Doc:      "Your Seerr address, which is how Loomarr downloads missing titles.",
			Detail:   "For example http://seerr:5055.",
			ShowWhen: map[string][]string{"requester.provider": {"seerr"}},
		},
		{
			Key: "seerr.api_key", EnvVar: "SEERR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "",
			Doc:      "Your Seerr API key.",
			ShowWhen: map[string][]string{"requester.provider": {"seerr"}},
		},
		// Where Sonarr and Radarr live is in their own connections below; these overrides only
		// shape what the direct requester asks them for.
		{
			Key: "sonarr.quality_profile", Label: "Sonarr quality profile override", EnvVar: "SONARR_QUALITY_PROFILE", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Sonarr quality profile new shows use, or blank for Sonarr's first.",
			Detail:   "Optional Sonarr quality profile (name or id). Blank = Sonarr's first profile.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "sonarr.root_folder", Label: "Sonarr root folder override", EnvVar: "SONARR_ROOT_FOLDER", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Sonarr folder new shows go in, or blank for Sonarr's first.",
			Detail:   "Optional Sonarr root folder path. Blank = Sonarr's first root folder.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.quality_profile", Label: "Radarr quality profile override", EnvVar: "RADARR_QUALITY_PROFILE", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Radarr quality profile new movies use, or blank for Radarr's first.",
			Detail:   "Optional Radarr quality profile (name or id). Blank = Radarr's first profile.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.root_folder", Label: "Radarr root folder override", EnvVar: "RADARR_ROOT_FOLDER", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Radarr folder new movies go in, or blank for Radarr's first.",
			Detail:   "Optional Radarr root folder path. Blank = Radarr's first root folder.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},

		// --- Connections: Sonarr and Radarr (#2151 decisions 2 and 29) ---
		// Their own connections, whichever requester is chosen: the library reads their folders
		// and what each file is, and they tell Loomarr about new downloads.
		{
			Key: "sonarr.url", EnvVar: "SONARR_URL", Group: GroupSonarr,
			Kind: KindURL, Default: "",
			Doc:    "Your Sonarr address.",
			Detail: "For example http://sonarr:8989. Loomarr learns your shows from it and hears about new episodes.",
		},
		{
			Key: "sonarr.api_key", EnvVar: "SONARR_API_KEY", Group: GroupSonarr,
			Kind: KindSecret, Default: "",
			Doc: "Your Sonarr API key (Settings → General in Sonarr).",
		},
		{
			Key: "radarr.url", EnvVar: "RADARR_URL", Group: GroupRadarr,
			Kind: KindURL, Default: "",
			Doc:    "Your Radarr address.",
			Detail: "For example http://radarr:7878. Loomarr learns your movies from it and hears about new ones.",
		},
		{
			Key: "radarr.api_key", EnvVar: "RADARR_API_KEY", Group: GroupRadarr,
			Kind: KindSecret, Default: "",
			Doc: "Your Radarr API key (Settings → General in Radarr).",
		},

		// --- Connections: Tunarr (§15, Phase 10) ---
		{
			Key: "tunarr.url", EnvVar: "TUNARR_URL", Group: GroupTunarr,
			Kind: KindURL, Default: "",
			Doc:    "Your Tunarr address, where Loomarr builds your channels.",
			Detail: "For example http://tunarr:8000.",
		},
		{
			Key: "tunarr.transcode_config_id", Label: "Transcode profile override", EnvVar: "TUNARR_TRANSCODE_CONFIG_ID", Group: GroupTunarr,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Which Tunarr transcode profile new channels use.",
			Detail: "Leave empty to use Tunarr's default.",
		},
		{
			// RE-SCOPED by V4 (§9.1), not duplicated. This was a Tunarr-group, Advanced
			// knob documented as "only needed for uploaded channel icons". Internal
			// playout makes it the base every SEGMENT request resolves against, so it
			// moves to Playout and stops being Advanced — "get it wrong and channels
			// appear in the guide but never play" is not an advanced failure mode.
			//
			// Deliberately ONE key, not a second `playout.public_url`: it is genuinely
			// the server's own public address, and both callers (icon fetch, segment
			// fetch) need the same value. Two keys could drift, and an operator would
			// have to know which one Live TV reads.
			Key: "server.public_url", Label: "Loomarr address", EnvVar: "SERVER_PUBLIC_URL", Group: GroupPlayout,
			Kind: KindURL, Default: "",
			Doc:    "The address your media server and Tunarr use to reach Loomarr's channel streams.",
			Detail: "Loomarr's own address as your media server and Tunarr can reach it, e.g. http://loomarr:8080. Your media server and Tunarr fetch every stream URL from this base, so a wrong value means channels appear in the guide there and never play. Loomarr's own in-app playback does not use it (it reaches itself on its listen address), and Current Health flags an address this server cannot reach. Also used for uploaded channel icons.",
		},

		{
			Key: "access.public_url", Label: "Recipient-facing Loomarr address", EnvVar: "ACCESS_PUBLIC_URL", Group: GroupGeneral,
			Kind: KindURL, Default: "", Validate: recipientHTTPOrigin,
			Doc:    "The address people open from an invitation or sign-in recovery link.",
			Detail: "The absolute browser address invitation and recovery recipients can reach, e.g. https://loomarr.example.com. General suggests the current browser origin when this is empty; save a different address when recipients reach Loomarr elsewhere. The server never infers it from request headers.",
		},
		{
			Key: "notifications.email.enabled", Label: "Send email notifications", EnvVar: "NOTIFICATIONS_EMAIL_ENABLED", Group: GroupNotifications,
			Kind: KindBool, Presentation: PresentationSwitch, Default: false, MigrationOnly: true,
			Doc:    "Send account invitations and recovery messages by email.",
			Detail: "An incomplete setup suppresses email without affecting copied links, QR codes, or direct account creation.",
		},
		{
			Key: "notifications.smtp.host", Label: "SMTP host", EnvVar: "NOTIFICATIONS_SMTP_HOST", Group: GroupNotifications,
			Kind: KindString, Default: "", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The mail server Loomarr sends email through.",
			Detail: "Hostname of the SMTP submission server. Required when email delivery is enabled.",
		},
		{
			Key: "notifications.smtp.port", Label: "SMTP port", EnvVar: "NOTIFICATIONS_SMTP_PORT", Group: GroupNotifications,
			Kind: KindInt, Default: 587, MigrationOnly: true, Validate: smtpPort, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc: "Port of the SMTP submission server, from 1 through 65535.",
		},
		{
			Key: "notifications.smtp.security", Label: "SMTP security", EnvVar: "NOTIFICATIONS_SMTP_SECURITY", Group: GroupNotifications,
			Kind: KindEnum, Enum: []EnumOption{
				opt("starttls", "STARTTLS (required)"),
				opt("tls", "TLS"),
				opt("none", "None (insecure local relay)"),
			},
			Default: "starttls", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "How the connection to the mail server is encrypted.",
			Detail: "STARTTLS requires encryption and never downgrades; TLS connects encrypted immediately. None is only for an explicitly trusted local relay. Certificate verification is always enabled.",
		},
		{
			Key: "notifications.smtp.username", Label: "SMTP username", EnvVar: "NOTIFICATIONS_SMTP_USERNAME", Group: GroupNotifications,
			Kind: KindString, Default: "", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The username for signing in to the mail server.",
			Detail: "Username for SMTP authentication. Leave empty only for an unauthenticated relay.",
		},
		{
			Key: "notifications.smtp.password", Label: "SMTP password", EnvVar: "NOTIFICATIONS_SMTP_PASSWORD", Group: GroupNotifications,
			Kind: KindSecret, Default: "", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The password for signing in to the mail server.",
			Detail: "Password for SMTP authentication. It is write-only, masked on read, and must remain empty for an unauthenticated relay.",
		},
		{
			Key: "notifications.email.from_address", Label: "Sender address", EnvVar: "NOTIFICATIONS_EMAIL_FROM_ADDRESS", Group: GroupNotifications,
			Kind: KindString, Default: "", MigrationOnly: true, Validate: mailboxAddress, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The email address Loomarr sends from.",
			Detail: "Mailbox Loomarr sends from, such as loomarr@example.com. Required when email delivery is enabled.",
		},
		{
			Key: "notifications.email.from_name", Label: "Sender name", EnvVar: "NOTIFICATIONS_EMAIL_FROM_NAME", Group: GroupNotifications,
			Kind: KindString, Default: "Loomarr", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc: "Display name shown beside the sender address.",
		},
	}
}

package config

type Settings struct {
	DBPath            string `json:"db_path" yaml:"db_path"`
	AnkiAccount       string `json:"anki_account" yaml:"anki_account"`
	HTTPAddr          string `json:"http_addr" yaml:"http_addr"`
	BackupKeepLastN   int    `json:"backup_keep_last_n" yaml:"backup_keep_last_n"`
	Workers           int    `json:"workers" yaml:"workers"`
	ForceApply        bool   `json:"force_apply" yaml:"force_apply"`
	Verbose           bool   `json:"verbose" yaml:"verbose"`
	FullDiff          bool   `json:"full_diff" yaml:"full_diff"`
	ReportFile        string `json:"report_file" yaml:"report_file"`
	ConfigPath        string `json:"-" yaml:"-"`
	DefaultPageSize   int    `json:"default_page_size" yaml:"default_page_size"`
	PragmaBusyTimeout int    `json:"pragma_busy_timeout" yaml:"pragma_busy_timeout"`
	PragmaJournalMode string `json:"pragma_journal_mode" yaml:"pragma_journal_mode"`
	PragmaSynchronous string `json:"pragma_synchronous" yaml:"pragma_synchronous"`
}

// Package usagesnapshot writes one row of usage numbers per workspace and UTC
// day to platform_usage_daily (platform spec 5.13). A nightly job computes the
// numbers for the day that just ended from tables that already exist. The
// platform console reads the table; the platform never computes the numbers.
package usagesnapshot

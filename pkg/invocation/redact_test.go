package invocation_test

import (
	"testing"

	"github.com/buildbarn/bb-portal/pkg/invocation"
	"github.com/stretchr/testify/require"
)

func TestRedactOptionValue(t *testing.T) {
	for _, tc := range []struct {
		name, option, value, want string
	}{
		{"RemoteHeader", "remote_header", "Authorization=Bearer abc", "Authorization=[REDACTED]"},
		{"BesHeader", "bes_header", "CF-Access-Client-Secret=s3cret", "CF-Access-Client-Secret=[REDACTED]"},
		{"RemoteCacheHeader", "remote_cache_header", "x-api-key=k", "x-api-key=[REDACTED]"},
		{"RemoteDownloaderHeader", "remote_downloader_header", "Authorization=Basic Zm9v", "Authorization=[REDACTED]"},
		{"RemoteExecHeader", "remote_exec_header", "x=y", "x=[REDACTED]"},
		{"HeaderWithoutEquals", "remote_header", "opaque", "[REDACTED]"},
		{"UrlUserinfo", "remote_cache", "grpcs://user:pass@cache.example.com:443", "grpcs://[REDACTED]@cache.example.com:443"},
		{"UrlTokenOnly", "bes_backend", "grpcs://token@bes.example.com", "grpcs://[REDACTED]@bes.example.com"},
		{"UrlWithoutUserinfo", "bes_backend", "grpcs://bes.example.com", "grpcs://bes.example.com"},
		{"AtSignInPathUntouched", "bes_results_url", "https://example.com/a@b", "https://example.com/a@b"},
		{"RepoLabelUntouched", "override_repository", "foo=@bar//baz", "foo=@bar//baz"},
		{"BuildMetadataUntouched", "build_metadata", "user_ldap=alice", "user_ldap=alice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, invocation.RedactOptionValue(tc.option, tc.value))
		})
	}
}

func TestRedactRawOptions(t *testing.T) {
	t.Run("EqualsForm", func(t *testing.T) {
		require.Equal(t,
			[]string{
				"--remote_header=Authorization=[REDACTED]",
				"--bes_header=CF-Access-Client-Id=[REDACTED]",
				"--bes_backend=grpcs://[REDACTED]@bes.example.com",
				"--build_metadata=user_ldap=alice",
				"--keep_going",
			},
			invocation.RedactRawOptions([]string{
				"--remote_header=Authorization=Bearer abc",
				"--bes_header=CF-Access-Client-Id=id123",
				"--bes_backend=grpcs://u:p@bes.example.com",
				"--build_metadata=user_ldap=alice",
				"--keep_going",
			}))
	})

	t.Run("SpaceSeparatedForm", func(t *testing.T) {
		require.Equal(t,
			[]string{"--remote_header", "Authorization=[REDACTED]", "//foo:bar"},
			invocation.RedactRawOptions([]string{"--remote_header", "Authorization=Bearer abc", "//foo:bar"}))
	})

	t.Run("Nil", func(t *testing.T) {
		require.Nil(t, invocation.RedactRawOptions(nil))
	})
}

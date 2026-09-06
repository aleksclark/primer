package broker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCallbackAttributionExhaustionIsSafeAndDoesNotChangeBudgets(t *testing.T) {
	for _, stage := range []string{"tuple_mapping", "issuance"} {
		t.Run(stage, func(t *testing.T) {
			pool := testutil.DB(t)
			ctx := context.Background()
			project, org, member, session := uniqueFixture()
			artifact := uniqueID("artifact")
			marker := "DO_NOT_LOG_sensitive_lower_level_value"
			provider := scripted(t, brokerprovider.Fixture{Artifact: artifact, Method: brokerprovider.MethodEmailMagicLink, Outcome: brokerprovider.OutcomeAuthenticated, ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session, ExpiresAt: time.Now().Add(time.Hour)})
			svc := newService(t, provider)
			client := uniqueID("client")
			redirect, resource, audience := registerClient(t, client)
			state := uniqueState("private-state")
			start, err := svc.Authorize(ctx, authorizeReq(client, redirect, resource, audience, state))
			require.NoError(t, err)
			suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
			seq := "diag_seq_" + suffix
			fn := "diag_fn_" + suffix
			trigger := "diag_trigger_" + suffix
			table, column := "stytch_mappings", "project_id"
			wantAttempts, wantCalls, wantTotal := 3, 5, 15
			if stage == "issuance" {
				table, column = "provider_session_associations", "provider_project_id"
				wantAttempts, wantCalls, wantTotal = 8, 0, 8
			}
			_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE SEQUENCE %s; CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.%s='%s' THEN PERFORM nextval('%s'); RAISE EXCEPTION '%s' USING ERRCODE='40001'; END IF; RETURN NEW; END; $$; CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s()`, seq, fn, column, project, seq, marker, trigger, table, fn))
			require.NoError(t, err)
			t.Cleanup(func() {
				_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s; DROP FUNCTION IF EXISTS %s(); DROP SEQUENCE IF EXISTS %s`, trigger, table, fn, seq))
			})
			_, err = svc.CompleteCallback(ctx, broker.CallbackInput{CookieValue: start.CookieValue, Artifact: artifact})
			require.Error(t, err)
			visible := "broker callback: tuple mapping unavailable"
			if stage == "issuance" {
				visible = "broker callback: issuance failed"
			}
			require.Equal(t, visible, err.Error())
			require.False(t, broker.IsRedirectable(err))
			d := broker.CallbackFailureDetails(err)
			require.Equal(t, stage, d.Stage())
			require.Equal(t, "serialization_conflict", d.Class())
			require.Equal(t, wantAttempts, d.Attempts())
			require.Equal(t, wantAttempts, d.Limit())
			require.Equal(t, wantCalls, d.MappingCalls())
			require.True(t, d.Exhausted())
			var actual int
			require.NoError(t, pool.QueryRow(ctx, `SELECT last_value FROM `+seq).Scan(&actual))
			require.Equal(t, wantTotal, actual, "existing retry budgets must be unchanged")
			raw, e := json.Marshal(d)
			require.NoError(t, e)
			for _, secret := range []string{marker, artifact, start.CookieValue, project, org, member, session, client, state, redirect} {
				if strings.Contains(string(raw), secret) || strings.Contains(d.String(), secret) {
					t.Fatal("diagnostic leaked sensitive metadata")
				}
			}
			var codes int
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, start.TransactionID).Scan(&codes))
			require.Zero(t, codes)
		})
	}
}

func TestCallbackAttributionUnknownCauseIsFixedUnknown(t *testing.T) {
	marker := "DO_NOT_LOG_account_cookie_SQLSTATE_40001"
	d := broker.CallbackFailureDetails(errors.New(marker))
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	require.Equal(t, "unknown", d.Stage())
	require.Equal(t, "unknown", d.Class())
	require.Zero(t, d.Attempts())
	require.False(t, d.Exhausted())
	if strings.Contains(string(raw), marker) || strings.Contains(d.String(), marker) {
		t.Fatal("unknown error text escaped the projection")
	}
}

func scripted(t *testing.T, f brokerprovider.Fixture) *brokerprovider.ScriptedProvider {
	t.Helper()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{Fixtures: []brokerprovider.Fixture{f}})
	require.NoError(t, err)
	return p
}

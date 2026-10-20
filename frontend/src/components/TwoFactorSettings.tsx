import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import QRCode from 'qrcode';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import SecurityIcon from '@mui/icons-material/Security';
import AppDialog from './AppDialog';
import {
  TwoFactorSetup,
  TwoFactorStatus,
  disableTwoFactor,
  enableTwoFactor,
  getTwoFactorStatus,
  regenerateRecoveryCodes,
  setupTwoFactor,
} from '../api/twoFactor';
import { ApiError } from '../api/client';
import { useSnackbar } from '../context/SnackbarContext';

type ConfirmMode = 'disable' | 'codes';

function QrImage({ uri, alt }: { uri: string; alt: string }) {
  const [src, setSrc] = useState('');
  useEffect(() => {
    let cancelled = false;
    // SVG text made by the library from our own URI; shown through an <img>, so nothing in it can run.
    QRCode.toString(uri, { type: 'svg', margin: 1, width: 200 })
      .then((svg) => {
        if (!cancelled) setSrc(`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [uri]);
  if (!src) return <CircularProgress size={24} />;
  return <Box component="img" src={src} alt={alt} sx={{ width: 200, height: 200, bgcolor: 'common.white' }} />;
}

function RecoveryCodes({ codes, onDone }: { codes: string[]; onDone: () => void }) {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(codes.join('\n'));
      showSuccess(t('twoFactor.codes.copied'));
    } catch {
      // clipboard not available (insecure origin): the codes are on screen to copy by hand
    }
  };
  return (
    <>
      <DialogTitle>{t('twoFactor.codes.title')}</DialogTitle>
      <DialogContent>
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('twoFactor.codes.warning')}
        </Alert>
        <Box
          component="ul"
          aria-label={t('twoFactor.codes.title')}
          sx={{
            listStyle: 'none',
            p: 0,
            m: 0,
            display: 'grid',
            gridTemplateColumns: 'repeat(2, 1fr)',
            gap: 0.5,
            fontFamily: 'monospace',
            fontSize: '1.1rem',
          }}
        >
          {codes.map((code) => (
            <li key={code}>{code}</li>
          ))}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={copy}>{t('twoFactor.codes.copy')}</Button>
        <Button variant="contained" onClick={onDone}>
          {t('twoFactor.codes.done')}
        </Button>
      </DialogActions>
    </>
  );
}

export default function TwoFactorSettings() {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const [status, setStatus] = useState<TwoFactorStatus | null>(null);
  const [loadError, setLoadError] = useState('');

  // Enrolment dialog
  const [setupOpen, setSetupOpen] = useState(false);
  const [setup, setSetup] = useState<TwoFactorSetup | null>(null);
  const [setupCode, setSetupCode] = useState('');
  const [setupPassword, setSetupPassword] = useState('');
  const [setupError, setSetupError] = useState('');
  const [busy, setBusy] = useState(false);

  // Shown after enabling or after new codes were made; the codes are never returned again.
  const [codes, setCodes] = useState<string[] | null>(null);

  // Password + code dialog (turn off, new recovery codes)
  const [confirmMode, setConfirmMode] = useState<ConfirmMode | null>(null);
  const [password, setPassword] = useState('');
  const [confirmCode, setConfirmCode] = useState('');
  const [useRecovery, setUseRecovery] = useState(false);
  const [confirmError, setConfirmError] = useState('');

  // Server messages are English; show a translated text for the cases the UI knows.
  const errorText = (err: unknown, fallback: string) =>
    err instanceof ApiError && err.status === 429 ? t('twoFactor.locked') : fallback;

  const load = useCallback(async () => {
    try {
      setStatus(await getTwoFactorStatus());
      setLoadError('');
    } catch {
      setLoadError(t('twoFactor.loadError'));
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const openSetup = async () => {
    setSetupOpen(true);
    setSetup(null);
    setSetupCode('');
    setSetupPassword('');
    setSetupError('');
    try {
      setSetup(await setupTwoFactor());
    } catch (err) {
      setSetupError(errorText(err, t('twoFactor.setup.error')));
    }
  };

  const closeSetup = () => {
    setSetupOpen(false);
    setSetup(null);
    setCodes(null);
    load();
  };

  const submitSetup = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setSetupError('');
    try {
      setCodes(await enableTwoFactor(setupPassword, setupCode));
    } catch (err) {
      if (err instanceof ApiError && err.status === 400 && err.details?.['field'] === 'password') {
        setSetupError(t('twoFactor.confirm.wrongPassword'));
      } else if (err instanceof ApiError && err.status === 400) {
        setSetupError(t('twoFactor.setup.invalid'));
      } else {
        setSetupError(errorText(err, t('twoFactor.actionError')));
      }
    } finally {
      setBusy(false);
    }
  };

  const openConfirm = (mode: ConfirmMode) => {
    setConfirmMode(mode);
    setPassword('');
    setConfirmCode('');
    setUseRecovery(false);
    setConfirmError('');
  };

  const closeConfirm = () => {
    setConfirmMode(null);
    setPassword('');
    setConfirmCode('');
  };

  const submitConfirm = async (event: FormEvent) => {
    event.preventDefault();
    if (!confirmMode) return;
    setBusy(true);
    setConfirmError('');
    const confirmation = {
      password,
      ...(useRecovery ? { recovery_code: confirmCode } : { code: confirmCode }),
    };
    try {
      if (confirmMode === 'disable') {
        await disableTwoFactor(confirmation);
        showSuccess(t('twoFactor.disabledSuccess'));
        closeConfirm();
        await load();
      } else {
        const fresh = await regenerateRecoveryCodes(confirmation);
        closeConfirm();
        setCodes(fresh);
        await load();
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 400 && err.details?.['field'] === 'password') {
        setConfirmError(t('twoFactor.confirm.wrongPassword'));
      } else if (err instanceof ApiError && err.status === 400) {
        setConfirmError(t('twoFactor.confirm.invalid'));
      } else {
        setConfirmError(errorText(err, t('twoFactor.actionError')));
      }
    } finally {
      setBusy(false);
    }
  };

  // Single sign-on only accounts: the identity provider is their second factor.
  if (status && !status.available) return null;

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <SecurityIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('twoFactor.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          {t('twoFactor.description')}
        </Typography>

        {loadError && <Alert severity="error">{loadError}</Alert>}
        {!status && !loadError && <CircularProgress size={24} />}
        {status && (
          <Stack spacing={1.5}>
            <Typography variant="body1">
              {status.enabled ? t('twoFactor.statusOn') : t('twoFactor.statusOff')}
            </Typography>
            {status.enabled && (
              <Typography variant="body2" color="text.secondary">
                {t('twoFactor.recoveryRemaining', { remaining: status.recovery_codes_remaining })}
              </Typography>
            )}
            <Stack direction="row" spacing={1}>
              {status.enabled ? (
                <>
                  <Button
                    size="small"
                    variant="outlined"
                    onClick={() => {
                      openConfirm('codes');
                    }}
                  >
                    {t('twoFactor.newCodes')}
                  </Button>
                  <Button
                    size="small"
                    variant="outlined"
                    color="error"
                    onClick={() => {
                      openConfirm('disable');
                    }}
                  >
                    {t('twoFactor.disable')}
                  </Button>
                </>
              ) : status.server_ready ? (
                <Button size="small" variant="contained" onClick={openSetup}>
                  {t('twoFactor.enable')}
                </Button>
              ) : (
                <Alert severity="info">{t('twoFactor.needsServerKey')}</Alert>
              )}
            </Stack>
          </Stack>
        )}
      </CardContent>

      <AppDialog open={setupOpen} onClose={codes ? undefined : closeSetup} maxWidth="xs" fullWidth>
        {codes ? (
          <RecoveryCodes codes={codes} onDone={closeSetup} />
        ) : (
          <form onSubmit={submitSetup}>
            <DialogTitle>{t('twoFactor.setup.title')}</DialogTitle>
            <DialogContent>
              <Stack spacing={2} sx={{ mt: 0.5 }}>
                {setupError && <Alert severity="error">{setupError}</Alert>}
                {setup && (
                  <>
                    <Typography variant="body2">{t('twoFactor.setup.scan')}</Typography>
                    <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                      <QrImage uri={setup.otpauth_uri} alt={t('twoFactor.setup.qrAlt')} />
                    </Box>
                    <Typography variant="body2">{t('twoFactor.setup.key')}</Typography>
                    <TextField
                      label={t('twoFactor.setup.keyLabel')}
                      value={setup.secret}
                      fullWidth
                      size="small"
                      slotProps={{ input: { readOnly: true, sx: { fontFamily: 'monospace' } } }}
                    />
                    <TextField
                      label={t('twoFactor.confirm.password')}
                      type="password"
                      value={setupPassword}
                      onChange={(e) => {
                        setSetupPassword(e.target.value);
                      }}
                      required
                      fullWidth
                      size="small"
                      autoComplete="current-password"
                    />
                    <TextField
                      label={t('twoFactor.setup.code')}
                      value={setupCode}
                      onChange={(e) => {
                        setSetupCode(e.target.value);
                      }}
                      required
                      fullWidth
                      size="small"
                      autoComplete="one-time-code"
                      slotProps={{ htmlInput: { inputMode: 'numeric' } }}
                    />
                  </>
                )}
                {!setup && !setupError && <CircularProgress size={24} />}
              </Stack>
            </DialogContent>
            <DialogActions>
              <Button onClick={closeSetup} disabled={busy}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" variant="contained" disabled={busy || !setup}>
                {busy ? t('twoFactor.setup.confirming') : t('twoFactor.setup.confirm')}
              </Button>
            </DialogActions>
          </form>
        )}
      </AppDialog>

      <AppDialog open={confirmMode !== null} onClose={closeConfirm} maxWidth="xs" fullWidth>
        <form onSubmit={submitConfirm}>
          <DialogTitle>
            {confirmMode === 'disable' ? t('twoFactor.confirm.disableTitle') : t('twoFactor.confirm.codesTitle')}
          </DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 0.5 }}>
              <Typography variant="body2">
                {confirmMode === 'disable'
                  ? t('twoFactor.confirm.disableMessage')
                  : t('twoFactor.confirm.codesMessage')}
              </Typography>
              {confirmError && <Alert severity="error">{confirmError}</Alert>}
              <TextField
                label={t('twoFactor.confirm.password')}
                type="password"
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                }}
                required
                fullWidth
                size="small"
                autoComplete="current-password"
              />
              <TextField
                label={useRecovery ? t('twoFactor.confirm.recoveryCode') : t('twoFactor.confirm.code')}
                value={confirmCode}
                onChange={(e) => {
                  setConfirmCode(e.target.value);
                }}
                required
                fullWidth
                size="small"
                autoComplete="one-time-code"
                slotProps={{ htmlInput: { inputMode: useRecovery ? 'text' : 'numeric' } }}
              />
              <Button
                size="small"
                sx={{ alignSelf: 'flex-start' }}
                onClick={() => {
                  setUseRecovery(!useRecovery);
                  setConfirmCode('');
                }}
              >
                {useRecovery ? t('twoFactor.confirm.useApp') : t('twoFactor.confirm.useRecovery')}
              </Button>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeConfirm} disabled={busy}>
              {t('common.cancel')}
            </Button>
            <Button
              type="submit"
              variant="contained"
              color={confirmMode === 'disable' ? 'error' : 'primary'}
              disabled={busy}
            >
              {confirmMode === 'disable' ? t('twoFactor.confirm.disableSubmit') : t('twoFactor.confirm.codesSubmit')}
            </Button>
          </DialogActions>
        </form>
      </AppDialog>

      {/* Recovery codes made from the settings card (not during enrolment) */}
      <AppDialog open={codes !== null && !setupOpen} maxWidth="xs" fullWidth>
        {codes && (
          <RecoveryCodes
            codes={codes}
            onDone={() => {
              setCodes(null);
            }}
          />
        )}
      </AppDialog>
    </Card>
  );
}

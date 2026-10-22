import { useEffect, useRef, useState } from 'react';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Pagination,
  Paper,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
  Link,
} from '@mui/material';
import {
  ActivityResult,
  ContactResult,
  MAX_PAGE,
  MAX_TERM,
  NoteResult,
  PAGE_SIZE,
  SearchKind,
  SearchResults,
  useSearchResults,
} from './hooks/useSearchResults';
import { useDateFormat } from './DateFormatProvider';
import { activitiesPath, contactPath, Marked, notePath, useFieldLabel } from './components/SearchMarks';

const KINDS: SearchKind[] = ['contacts', 'notes', 'activities'];

type Tab = 'all' | SearchKind;
function ContactRow({ result }: { result: ContactResult }) {
  const fieldLabel = useFieldLabel();
  const label =
    result.matched_field && result.matched_field !== 'name'
      ? fieldLabel(result.matched_field, result.matched_field_name)
      : '';
  return (
    <Box component="li" sx={{ py: 1 }}>
      <Link component={RouterLink} to={contactPath(result.id)} underline="hover" variant="subtitle1">
        <Marked segments={result.name} />
      </Link>
      {result.matched_field && result.matched_field !== 'name' && (
        <Typography variant="body2" color="text.secondary">
          {label && `${label}: `}
          <Marked segments={result.snippet} />
        </Typography>
      )}
    </Box>
  );
}

function NoteRow({ result, term }: { result: NoteResult; term: string }) {
  const { formatDate } = useDateFormat();
  return (
    <Box component="li" sx={{ py: 1 }}>
      <Link component={RouterLink} to={notePath(result.contact_id, term)} underline="hover" variant="body1">
        <Marked segments={result.snippet} />
      </Link>
      <Typography variant="caption" color="text.secondary" display="block">
        {formatDate(result.date)}
        {result.contact_name && ` · ${result.contact_name}`}
      </Typography>
    </Box>
  );
}

function ActivityRow({ result, term }: { result: ActivityResult; term: string }) {
  const { formatDateRange } = useDateFormat();
  return (
    <Box component="li" sx={{ py: 1 }}>
      <Link component={RouterLink} to={activitiesPath(term)} underline="hover" variant="subtitle1">
        <Marked segments={result.title} />
      </Link>
      <Typography variant="caption" color="text.secondary" display="block">
        {formatDateRange(result.date, result.end_date)}
        {(result.contacts ?? []).length > 0 && ` · ${(result.contacts ?? []).join(', ')}`}
      </Typography>
      {(result.snippet ?? []).length > 0 && (
        <Typography variant="body2" color="text.secondary">
          <Marked segments={result.snippet} />
        </Typography>
      )}
    </Box>
  );
}

function ResultList({ kind, results, term }: { kind: SearchKind; results: SearchResults; term: string }) {
  return (
    <Box component="ul" sx={{ listStyle: 'none', m: 0, p: 0 }}>
      {kind === 'contacts' && results.contacts.items.map((r) => <ContactRow key={r.id} result={r} />)}
      {kind === 'notes' && results.notes.items.map((r) => <NoteRow key={r.id} result={r} term={term} />)}
      {kind === 'activities' && results.activities.items.map((r) => <ActivityRow key={r.id} result={r} term={term} />)}
    </Box>
  );
}

export default function SearchPage() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const term = Array.from((params.get('q') ?? '').trim())
    .slice(0, MAX_TERM)
    .join('');
  const tabParam = params.get('tab');
  const tab: Tab = KINDS.find((k) => k === tabParam) ?? 'all';
  const page = Math.min(MAX_PAGE, Math.max(1, parseInt(params.get('page') ?? '1', 10) || 1));
  const { searchable, overview, paged, retry } = useSearchResults(term, tab, page);

  // A page beyond the last one (an old link, a typed address) shows the last page instead
  const pagedTotal = tab !== 'all' && paged.state === 'ready' ? paged.data[tab].total : 0;
  const lastPage = Math.max(1, Math.ceil(pagedTotal / PAGE_SIZE));
  useEffect(() => {
    if (tab !== 'all' && pagedTotal > 0 && page > lastPage) {
      const next: Record<string, string> = { q: term, tab };
      if (lastPage > 1) next['page'] = String(lastPage);
      setParams(next, { replace: true });
    }
  }, [tab, pagedTotal, page, lastPage, term, setParams]);

  const [input, setInput] = useState(term);
  const focusTab = useRef(false);

  useEffect(() => {
    setInput(term);
  }, [term]);

  useEffect(() => {
    if (focusTab.current) {
      focusTab.current = false;
      document.getElementById(`search-tab-${tab}`)?.focus();
    }
  }, [tab]);

  const submit = (event: React.SyntheticEvent) => {
    event.preventDefault();
    const next = input.trim();
    if (next) setParams({ q: next });
  };

  const goTo = (next: Tab, nextPage = 1) => {
    const p: Record<string, string> = { q: term };
    if (next !== 'all') p['tab'] = next;
    if (nextPage > 1) p['page'] = String(nextPage);
    setParams(p);
  };

  const totals = overview.state === 'ready' ? overview.data : null;
  const count = (k: SearchKind) => (totals ? ` (${String(totals[k].total)})` : '');
  const allTotal = totals ? totals.contacts.total + totals.notes.total + totals.activities.total : null;
  const names: Record<SearchKind, string> = {
    contacts: t('search.contacts'),
    notes: t('search.notes'),
    activities: t('search.activities'),
  };

  const failed = (tab === 'all' ? overview : paged).state === 'error';
  const loading = (tab === 'all' ? overview : paged).state === 'loading';

  return (
    <Box>
      <Typography variant="h4" component="h1" gutterBottom>
        {t('search.pageTitle')}
      </Typography>
      <Paper component="form" onSubmit={submit} role="search" sx={{ p: 1.5, mb: 2 }}>
        <TextField
          fullWidth
          size="small"
          type="search"
          label={t('search.submit')}
          value={input}
          onChange={(e) => {
            setInput(e.target.value);
          }}
        />
      </Paper>

      {!searchable ? (
        <Typography color="text.secondary">{t('search.tooShort')}</Typography>
      ) : (
        <>
          <Tabs
            value={tab}
            onChange={(_, value: Tab) => {
              goTo(value);
            }}
            aria-label={t('search.groups')}
            variant="scrollable"
            scrollButtons="auto"
            sx={{ mb: 2 }}
          >
            <Tab
              value="all"
              id="search-tab-all"
              aria-controls="search-panel"
              label={`${t('search.tabAll')}${allTotal === null ? '' : ` (${String(allTotal)})`}`}
            />
            {KINDS.map((k) => (
              <Tab
                key={k}
                value={k}
                id={`search-tab-${k}`}
                aria-controls="search-panel"
                label={`${names[k]}${count(k)}`}
              />
            ))}
          </Tabs>

          <Box role="tabpanel" id="search-panel" aria-labelledby={`search-tab-${tab}`} aria-busy={loading}>
            {loading && (
              <Box display="flex" justifyContent="center" mt={4}>
                <CircularProgress aria-label={t('search.searching')} />
              </Box>
            )}
            {failed && (
              <Alert
                severity="error"
                action={
                  <Button color="inherit" size="small" onClick={retry}>
                    {t('search.retry')}
                  </Button>
                }
              >
                {t('search.error')}
              </Alert>
            )}

            {tab === 'all' && overview.state === 'ready' && (
              <AllTab
                results={overview.data}
                term={term}
                names={names}
                onShowAll={(k) => {
                  focusTab.current = true;
                  goTo(k);
                }}
              />
            )}

            {tab !== 'all' && paged.state === 'ready' && (
              <>
                {paged.data[tab].total === 0 ? (
                  <Typography color="text.secondary">{t('search.noResults', { query: term })}</Typography>
                ) : (
                  <>
                    {tab === 'contacts' && (
                      <Button
                        component={RouterLink}
                        to={`/contacts?search=${encodeURIComponent(term)}`}
                        size="small"
                        sx={{ mb: 1 }}
                      >
                        {t('search.showInContactList')}
                      </Button>
                    )}
                    <ResultList kind={tab} results={paged.data} term={term} />
                  </>
                )}
                {paged.data[tab].total > PAGE_SIZE && (
                  <Stack alignItems="center" sx={{ mt: 2 }}>
                    <Pagination
                      count={Math.ceil(paged.data[tab].total / PAGE_SIZE)}
                      page={page}
                      onChange={(_, value) => {
                        goTo(tab, value);
                      }}
                    />
                  </Stack>
                )}
              </>
            )}
          </Box>
        </>
      )}
    </Box>
  );
}

function AllTab({
  results,
  term,
  names,
  onShowAll,
}: {
  results: SearchResults;
  term: string;
  names: Record<SearchKind, string>;
  onShowAll: (kind: SearchKind) => void;
}) {
  const { t } = useTranslation();
  const total = results.contacts.total + results.notes.total + results.activities.total;
  if (total === 0) {
    return <Typography color="text.secondary">{t('search.noResults', { query: term })}</Typography>;
  }
  return (
    <Stack spacing={3}>
      {KINDS.filter((k) => results[k].total > 0).map((k) => (
        <Box key={k} component="section" aria-labelledby={`search-group-${k}`}>
          <Typography variant="h6" component="h2" id={`search-group-${k}`}>
            {names[k]} ({results[k].total})
          </Typography>
          <ResultList kind={k} results={results} term={term} />
          {results[k].total > results[k].items.length && (
            <Button
              size="small"
              onClick={() => {
                onShowAll(k);
              }}
            >
              {t('search.showAllCount', { count: results[k].total })}
            </Button>
          )}
        </Box>
      ))}
    </Stack>
  );
}

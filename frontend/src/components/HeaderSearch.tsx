import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Autocomplete, Box, IconButton, InputAdornment, ListSubheader, TextField, Typography } from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import ClearIcon from '@mui/icons-material/Clear';
import { searchAll, SearchResults } from '../api/search';
import { activitiesPath, contactPath, Marked, notePath, searchPath, useFieldLabel } from './SearchMarks';

const MIN_CHARS = 2;
const DEBOUNCE_MS = 300;
const CONTACT_LIMIT = 5;
const OTHER_LIMIT = 3;

type GroupKey = 'contacts' | 'notes' | 'activities' | '';

type Option =
  | { kind: 'contact'; group: GroupKey; result: SearchResults['contacts']['items'][number] }
  | { kind: 'note'; group: GroupKey; result: SearchResults['notes']['items'][number] }
  | { kind: 'activity'; group: GroupKey; result: SearchResults['activities']['items'][number] }
  | { kind: 'status'; group: GroupKey; text: string }
  | { kind: 'all'; group: GroupKey; text: string };

const optionKey = (o: Option) =>
  o.kind === 'status' || o.kind === 'all' ? o.kind : `${o.kind}-${String(o.result.id)}`;

interface HeaderSearchProps {
  enabled: boolean;
}

export default function HeaderSearch({ enabled }: HeaderSearchProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const [query, setQuery] = useState('');
  const [answer, setAnswer] = useState<{ term: string; results: SearchResults } | null>(null);
  const [loading, setLoading] = useState(false);
  const latest = useRef(0);

  const term = query.trim();
  const results = answer?.results ?? null;
  const fresh = answer?.term === term;
  const active = enabled && term.length >= MIN_CHARS;

  useEffect(() => {
    latest.current += 1;
    const request = latest.current;
    if (!active) {
      setAnswer(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    const timer = setTimeout(() => {
      void searchAll(term, { limit: CONTACT_LIMIT })
        .then((found) => {
          if (request === latest.current) setAnswer({ term, results: found });
        })
        .catch((err: unknown) => {
          console.error('Search error:', err);
          if (request === latest.current) setAnswer(null);
        })
        .finally(() => {
          if (request === latest.current) setLoading(false);
        });
    }, DEBOUNCE_MS);
    return () => {
      clearTimeout(timer);
    };
  }, [term, active]);

  // The header search is a quick jump, not a page filter: leaving the page discards it
  useEffect(() => {
    setQuery('');
  }, [location.pathname]);

  const reset = () => {
    setQuery('');
  };

  const showAll = (text: string) => {
    if (text.trim()) {
      void navigate(searchPath(text.trim()));
      reset();
    }
  };

  const options: Option[] = [];
  if (active) {
    if (results) {
      results.contacts.items.slice(0, CONTACT_LIMIT).forEach((result) => {
        options.push({ kind: 'contact', group: 'contacts', result });
      });
      results.notes.items.slice(0, OTHER_LIMIT).forEach((result) => {
        options.push({ kind: 'note', group: 'notes', result });
      });
      results.activities.items.slice(0, OTHER_LIMIT).forEach((result) => {
        options.push({ kind: 'activity', group: 'activities', result });
      });
    }
    const total = results ? results.contacts.total + results.notes.total + results.activities.total : 0;
    if (loading && !results) {
      options.unshift({ kind: 'status', group: '', text: t('search.searching') });
    } else if (fresh && total === 0) {
      options.unshift({ kind: 'status', group: '', text: t('search.noResults', { query: term }) });
    }
    options.push({ kind: 'all', group: '', text: t('search.showAll', { query: term }) });
  }

  const totals = (group: GroupKey): number => (group && results ? results[group].total : 0);

  const open = (option: Option) => {
    switch (option.kind) {
      case 'contact':
        void navigate(contactPath(option.result.id));
        break;
      case 'note':
        void navigate(notePath(option.result.contact_id, term));
        break;
      case 'activity':
        void navigate(activitiesPath(term));
        break;
      case 'all':
        showAll(term);
        return;
      case 'status':
        return;
    }
    reset();
  };

  return (
    <Autocomplete<Option, false, false, true>
      freeSolo
      clearOnEscape
      size="small"
      options={options}
      value={null}
      groupBy={(option) => option.group}
      getOptionKey={(option) => (typeof option === 'string' ? option : optionKey(option))}
      filterOptions={(all) => all}
      getOptionLabel={(option) => (typeof option === 'string' ? option : '')}
      getOptionDisabled={(option) => option.kind === 'status'}
      loading={loading}
      loadingText={t('search.searching')}
      inputValue={query}
      onInputChange={(_, value, reason) => {
        if (reason !== 'reset') setQuery(value);
      }}
      onChange={(_, value) => {
        if (typeof value === 'string') {
          showAll(value);
        } else if (value) {
          open(value);
        }
      }}
      renderGroup={(params) =>
        params.group ? (
          <li key={params.key}>
            <ListSubheader component="div" sx={{ lineHeight: '32px', fontWeight: 600 }}>
              {t(`search.${params.group}`)} ({totals(params.group as GroupKey)})
            </ListSubheader>
            <ul style={{ padding: 0 }}>{params.children}</ul>
          </li>
        ) : (
          <li key={params.key}>
            <ul style={{ padding: 0 }}>{params.children}</ul>
          </li>
        )
      }
      renderOption={({ key, ...props }, option) => (
        <li key={key} {...props}>
          <OptionBody option={option} />
        </li>
      )}
      renderInput={(params) => (
        <TextField
          {...params}
          placeholder={t('search.placeholder')}
          variant="outlined"
          sx={{
            width: { xs: 150, sm: 200, md: 250 },
            mr: 2,
            '& .MuiOutlinedInput-root': {
              backgroundColor: 'rgba(255, 255, 255, 0.15)',
              '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.25)' },
              '& fieldset': { borderColor: 'rgba(255, 255, 255, 0.3)' },
              '&:hover fieldset': { borderColor: 'rgba(255, 255, 255, 0.5)' },
              '&.Mui-focused fieldset': { borderColor: 'rgba(255, 255, 255, 0.7)' },
            },
            '& .MuiInputBase-input': { color: 'white' },
            '& .MuiInputBase-input::placeholder': { color: 'rgba(255, 255, 255, 0.7)', opacity: 1 },
          }}
          slotProps={{
            input: {
              ...params.InputProps,
              startAdornment: (
                <InputAdornment position="start">
                  <IconButton
                    size="small"
                    aria-label={t('search.submit')}
                    onClick={() => {
                      showAll(query);
                    }}
                    sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.5 }}
                  >
                    <SearchIcon />
                  </IconButton>
                </InputAdornment>
              ),
              endAdornment: query ? (
                <InputAdornment position="end">
                  <IconButton
                    size="small"
                    aria-label={t('search.clear')}
                    onClick={reset}
                    sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.5 }}
                  >
                    <ClearIcon fontSize="small" />
                  </IconButton>
                </InputAdornment>
              ) : null,
            },
          }}
        />
      )}
    />
  );
}

function OptionBody({ option }: { option: Option }) {
  const fieldLabel = useFieldLabel();
  switch (option.kind) {
    case 'contact': {
      const c = option.result;
      return (
        <Box>
          <Typography variant="body1">
            <Marked segments={c.name} />
          </Typography>
          {c.matched_field && c.matched_field !== 'name' && (
            <Typography variant="caption" color="text.secondary">
              {fieldLabel(c.matched_field, c.matched_field_name) &&
                `${fieldLabel(c.matched_field, c.matched_field_name)}: `}
              <Marked segments={c.snippet} />
            </Typography>
          )}
        </Box>
      );
    }
    case 'note': {
      const n = option.result;
      return (
        <Box>
          {n.contact_name && (
            <Typography variant="caption" color="text.secondary" display="block">
              {n.contact_name}
            </Typography>
          )}
          <Typography variant="body2">
            <Marked segments={n.snippet} />
          </Typography>
        </Box>
      );
    }
    case 'activity': {
      const a = option.result;
      return (
        <Box>
          <Typography variant="body1">
            <Marked segments={a.title} />
          </Typography>
          {(a.snippet ?? []).length > 0 && (
            <Typography variant="caption" color="text.secondary">
              <Marked segments={a.snippet} />
            </Typography>
          )}
        </Box>
      );
    }
    default:
      return (
        <Typography variant="body2" color={option.kind === 'status' ? 'text.secondary' : 'primary'}>
          {option.text}
        </Typography>
      );
  }
}

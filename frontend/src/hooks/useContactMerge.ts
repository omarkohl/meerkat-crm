import { useState, useEffect, useCallback, useRef } from 'react';
import { getFullContact, mergeContacts } from '../api/merge';
import { planMerge, type MergeContact, type MergePlan } from '../utils/mergeContacts';
import { handleFetchError } from '../utils/errorHandler';

interface UseContactMergeResult {
  contacts: MergeContact[];
  loading: boolean;
  error: string | null;
  merging: boolean;
  merge: (targetId: number, choices: Record<string, string>) => Promise<boolean>;
  planFor: (targetId: number) => MergePlan | null;
}

export function useContactMerge(ids: number[], open: boolean): UseContactMergeResult {
  const [contacts, setContacts] = useState<MergeContact[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [merging, setMerging] = useState(false);
  const key = ids.join(',');
  const idsRef = useRef(ids);
  idsRef.current = ids;

  const load = useCallback(async (isCancelled: () => boolean = () => false) => {
    setLoading(true);
    setError(null);
    try {
      const loaded = await Promise.all(idsRef.current.map((id) => getFullContact(id)));
      if (!isCancelled()) setContacts(loaded);
    } catch (err) {
      if (!isCancelled()) setError(handleFetchError(err, 'loading contacts to merge'));
    } finally {
      if (!isCancelled()) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open || idsRef.current.length === 0) return;
    let cancelled = false;
    void load(() => cancelled);
    return () => {
      cancelled = true;
    };
  }, [key, open, load]);

  const planFor = useCallback(
    (targetId: number) => {
      const target = contacts.find((c) => c.ID === targetId);
      if (!target) return null;
      return planMerge(
        target,
        contacts.filter((c) => c.ID !== targetId),
      );
    },
    [contacts],
  );

  const merge = useCallback(
    async (targetId: number, choices: Record<string, string>) => {
      const plan = planFor(targetId);
      if (!plan) return false;
      setMerging(true);
      setError(null);
      try {
        await mergeContacts(plan, choices);
        return true;
      } catch (err) {
        await load();
        setError(handleFetchError(err, 'merging contacts'));
        return false;
      } finally {
        setMerging(false);
      }
    },
    [planFor, load],
  );

  return { contacts, loading, error, merging, merge, planFor };
}

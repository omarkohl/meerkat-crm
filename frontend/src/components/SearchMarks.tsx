import { useTranslation } from 'react-i18next';
import { Segment } from '../api/search';

const FIELD_LABELS: Record<string, string> = {
  email: 'contacts.email',
  phone: 'contacts.phone',
  organization: 'contacts.organization',
  job_title: 'contacts.jobTitle',
  department: 'contacts.department',
  role: 'contacts.role',
  middle_name: 'contacts.middleName',
  address: 'contacts.address',
  circles: 'contacts.circles',
  how_we_met: 'contacts.howWeMet',
  food_preference: 'contacts.foodPreference',
  work_information: 'contacts.workInformation',
  contact_information: 'contacts.contactInformation',
  note: 'search.noteField',
};

/** Text with the matched parts marked. */
export function Marked({ segments }: { segments: Segment[] | null }) {
  return (
    <>
      {(segments ?? []).map((s, i) =>
        s.match ? (
          <mark key={i} style={{ background: 'rgba(255, 213, 79, 0.6)', color: 'inherit' }}>
            {s.text}
          </mark>
        ) : (
          <span key={i}>{s.text}</span>
        ),
      )}
    </>
  );
}

/** The label of the contact field a result matched in, or '' for a name and for a field this version does not know. */
export function useFieldLabel() {
  const { t } = useTranslation();
  return (field: string, customName: string): string => {
    if (field === 'custom_field') return customName;
    const key = FIELD_LABELS[field];
    return key ? t(key) : '';
  };
}

export const contactPath = (id: number) => `/contacts/${String(id)}`;
export const notePath = (contactId: number | null, term: string) =>
  contactId ? contactPath(contactId) : `/notes?search=${encodeURIComponent(term)}`;
export const activitiesPath = (term: string) => `/activities?search=${encodeURIComponent(term)}`;
export const searchPath = (term: string) => `/search?q=${encodeURIComponent(term)}`;

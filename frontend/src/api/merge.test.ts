import { describe, test, expect, vi, beforeEach } from 'vitest';
import { mergeContacts } from './merge';
import { planMerge, type MergeContact } from '../utils/mergeContacts';
import * as contacts from './contacts';
import * as images from './images';
import * as relationships from './relationships';

vi.mock('./contacts');
vi.mock('./images');
vi.mock('./notes');
vi.mock('./reminders');
vi.mock('./activities');
vi.mock('./relationships');

const ref = (id: number) => ({ id, crop: null }) as images.ImageRef;

function contact(ID: number, overrides: Partial<MergeContact> = {}): MergeContact {
  return {
    ID,
    firstname: 'Anna',
    lastname: '',
    circles: [],
    emails: [],
    phones: [],
    addresses: [],
    urls: [],
    impps: [],
    custom_fields: {},
    notes: [],
    activities: [],
    reminders: [],
    relationships: [],
    images: [],
    ...overrides,
  } as MergeContact;
}

describe('mergeContacts photos', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(contacts.getContactProfilePicture).mockResolvedValue(new Blob(['x'], { type: 'image/jpeg' }));
    vi.mocked(images.uploadImage).mockImplementation(() => ({ promise: Promise.resolve(ref(99)), abort: vi.fn() }));
    vi.mocked(relationships.getIncomingRelationships).mockResolvedValue({ incoming_relationships: [] });
    vi.mocked(contacts.getContact).mockResolvedValue(contact(1) as never);
  });

  test('keeps the unmigrated photo of the target ahead of the images of a source', async () => {
    const target = contact(1, { photo: 'p.jpg' });
    const source = contact(2, { images: [ref(5)], main_image_id: 5 });
    await mergeContacts(planMerge(target, [source]), {});
    expect(contacts.setContactImages).toHaveBeenCalledWith(1, [99, 5]);
    expect(contacts.setMainImage).not.toHaveBeenCalled();
  });

  test('moves the unmigrated photo of a source', async () => {
    await mergeContacts(planMerge(contact(1), [contact(2, { photo: 'p.jpg' })]), {});
    expect(contacts.setContactImages).toHaveBeenCalledWith(1, [99]);
  });
});

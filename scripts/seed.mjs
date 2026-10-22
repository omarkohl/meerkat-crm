#!/usr/bin/env node
// Seeds a running dev backend (make dev) with a demo user and realistic data: contacts with
// every kind of field, notes, activities, reminders and relationships.
// Safe to run again: existing user and contacts are kept, and only new contacts get extras.
// Usage: make seed   (BACKEND_PORT / SEED_API_URL override the target)
const base = process.env.SEED_API_URL ?? `http://localhost:${process.env.BACKEND_PORT ?? 8080}/api/v1`;
const USER = { username: 'demo', email: 'demo@example.com', password: 'DemoPassword123!' };

let seed = 42;
const rand = () => {
  seed = (seed * 1664525 + 1013904223) % 2 ** 32;
  return seed / 2 ** 32;
};
const pick = (list) => list[Math.floor(rand() * list.length)];
const some = (list, n) => [...list].sort(() => rand() - 0.5).slice(0, n);
const int = (min, max) => min + Math.floor(rand() * (max - min + 1));
const pad = (n) => String(n).padStart(2, '0');
const isoDate = (d) => d.toISOString().slice(0, 10);
const daysFromNow = (days, hour = 9) => {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() + days);
  d.setUTCHours(hour, 0, 0, 0);
  return d.toISOString();
};

const FIRST = [
  'Alice',
  'Bob',
  'Carol',
  'David',
  'Eva',
  'Frank',
  'Grace',
  'Hannah',
  'Ivan',
  'Julia',
  'Karim',
  'Lena',
  'Marco',
  'Nadia',
  'Oscar',
  'Priya',
  'Quentin',
  'Rosa',
  'Sven',
  'Tara',
  'Uma',
  'Victor',
  'Wen',
  'Yara',
];
const LAST = [
  'Johnson',
  'Smith',
  'Williams',
  'Brown',
  'Müller',
  'Rossi',
  'García',
  'Dubois',
  'Nakamura',
  'Okafor',
  'Kowalski',
  'Haddad',
  'Larsen',
  'Patel',
  'Silva',
  'Novak',
  'Fischer',
  'Moreau',
  'Costa',
  'Andersson',
];
const CITIES = [
  ['Berlin', 'Germany', '10115'],
  ['Lyon', 'France', '69001'],
  ['Milan', 'Italy', '20121'],
  ['Madrid', 'Spain', '28001'],
  ['Austin', 'United States', '78701'],
  ['Toronto', 'Canada', 'M5V 2T6'],
];
const COMPANIES = [
  ['Acme Corp', 'Engineering', 'Software Engineer'],
  ['Globex', 'Sales', 'Account Manager'],
  ['Initech', 'Finance', 'Analyst'],
  ['Umbrella Health', 'Research', 'Scientist'],
  ['Stark Design', 'Product', 'Designer'],
  ['Wayne Logistics', 'Operations', 'Coordinator'],
];
const CIRCLES = ['Friends', 'Family', 'Work', 'Neighbours', 'Climbing club', 'Book club', 'University', 'Clients'];
const FOODS = ['Vegetarian', 'Vegan', 'No nuts, allergic', 'Loves spicy food', 'Gluten free', 'No pork'];
const MET = [
  'At a conference in 2019',
  'Through a mutual friend',
  'University flatmate',
  'Neighbour since we moved',
  'At a wedding',
];
const NOTES = [
  'Talked about their new job. Seems happy but stressed about the commute.',
  'Recommended a book: "The Overstory". Wants to borrow my copy.',
  'Is planning a trip next spring. Ask about it.',
  'Moved to a bigger flat. Housewarming party in autumn.',
  'Struggling with a knee injury, no running for now.',
  'Introduced me to their partner. Very friendly.',
  'Asked for feedback on a side project. Send thoughts by Friday.',
  'Birthday gift idea: good coffee beans.',
];
const ACTIVITIES = [
  ['Dinner', 'Italian place downtown', 'Great pasta, long talk'],
  ['Hike', 'Black Forest', 'Eight hours, sore legs the next day'],
  ['Coffee', 'Café near the station', 'Quick catch-up'],
  ['Board game night', 'My place', 'They won every round'],
  ['Phone call', '', 'Talked for an hour about the move'],
  ['Concert', 'City arena', 'Front row tickets'],
  ['Birthday party', 'Their garden', 'Brought the cake'],
];
const REMINDERS = [
  ['Call to ask how the move went', 'once'],
  ['Send birthday card', 'yearly'],
  ['Check in', 'monthly'],
  ['Catch up over coffee', 'quarterly'],
  ['Ask about the project', 'weekly'],
  ['Plan the summer trip', 'six-months'],
];
const REL_TYPES = ['Friend', 'Colleague', 'Neighbor', 'Sibling', 'Spouse', 'Partner', 'Manager', 'Best Friend'];

const handcrafted = [
  {
    firstname: 'Alice',
    lastname: 'Johnson',
    nickname: 'Ali',
    gender: 'female',
    emails: [
      { type: 'home', value: 'alice@example.com' },
      { type: 'work', value: 'alice.johnson@acme.example' },
    ],
    phones: [{ type: 'mobile', value: '+1 555-0101' }],
    birthday: '1990-03-15',
    circles: ['Friends', 'Work'],
    organization: 'Acme Corp',
    job_title: 'Engineering Manager',
    how_we_met: 'University flatmate',
    food_preference: 'Vegetarian',
    urls: [{ type: 'website', value: 'https://alice.example.com' }],
    custom_fields: { 'Favourite band': 'Radiohead', 'T-shirt size': 'M' },
  },
  { firstname: 'Bob', lastname: 'Smith', phones: [{ type: 'mobile', value: '+1 555-0102' }], circles: ['Family'] },
  {
    firstname: 'Carol',
    lastname: 'Williams',
    emails: [{ type: 'home', value: 'carol@example.com' }],
    birthday: '1985-07-22',
    anniversary: '2015-06-20',
    circles: ['Friends'],
    addresses: [
      { type: 'home', street: 'Hauptstraße 12', city: 'Berlin', region: '', postal: '10115', country: 'Germany' },
    ],
  },
  {
    firstname: 'Dr. Zoë',
    lastname: 'Okonkwo-Brandt',
    prefix: 'Dr.',
    middle_name: 'Amara',
    suffix: 'PhD',
    emails: [{ type: 'work', value: 'zoe@umbrella.example' }],
    birthday: '--06-30',
    circles: ['Work', 'University'],
    organization: 'Umbrella Health',
    department: 'Research',
    job_title: 'Principal Scientist',
    role: 'Mentor',
    note: 'Long name with special characters to test sorting and display.',
  },
  { firstname: 'Yuki', lastname: 'Tanaka', circles: [] },
  // Duplicates as they arise in real life: once typed into a mail client, once saved on the phone.
  {
    firstname: 'Katharina',
    lastname: 'Meier',
    emails: [{ type: 'work', value: 'katharina.meier@stadtwerke.example' }],
    birthday: '1988-11-14',
    organization: 'Stadtwerke Freiburg',
    circles: ['Work'],
  },
  {
    firstname: 'Kathi',
    lastname: 'Meyer',
    nickname: 'Kathi',
    emails: [{ type: 'home', value: 'kathi.meier@example.org' }],
    phones: [{ type: 'mobile', value: '+49 171 5550123' }],
    birthday: '1988-11-04',
    circles: ['Friends'],
    note: 'Met at the climbing gym.',
  },
  {
    firstname: 'Jonathan',
    lastname: 'Becker',
    phones: [{ type: 'mobile', value: '+49 160 5550177' }],
    addresses: [
      { type: 'home', street: 'Lindenstr. 8', city: 'Leipzig', region: '', postal: '04109', country: 'Germany' },
    ],
  },
  {
    firstname: 'Jonas',
    lastname: 'Becker',
    emails: [{ type: 'home', value: 'jonas.becker@example.net' }],
    phones: [{ type: 'mobile', value: '+49 160 5550177' }],
    birthday: '1992-02-19',
    job_title: 'Teacher',
    circles: ['Neighbours'],
  },
];

function generated(count) {
  const used = new Set(handcrafted.map((c) => `${c.firstname} ${c.lastname}`));
  const result = [];
  while (result.length < count) {
    const firstname = pick(FIRST);
    const lastname = pick(LAST);
    if (used.has(`${firstname} ${lastname}`)) continue;
    used.add(`${firstname} ${lastname}`);
    const slug = `${firstname}.${lastname}`
      .toLowerCase()
      .normalize('NFD')
      .replace(/[^a-z.]/g, '');
    const contact = { firstname, lastname, circles: some(CIRCLES, int(0, 3)) };
    if (rand() < 0.8) contact.emails = [{ type: pick(['home', 'work']), value: `${slug}@example.com` }];
    if (rand() < 0.6)
      contact.phones = [{ type: pick(['mobile', 'home', 'work']), value: `+49 170 ${int(1000000, 9999999)}` }];
    if (rand() < 0.7) contact.birthday = `${int(1960, 2002)}-${pad(int(1, 12))}-${pad(int(1, 28))}`;
    if (rand() < 0.15) contact.nickname = firstname.slice(0, 3);
    if (rand() < 0.4) {
      const [city, country, postal] = pick(CITIES);
      contact.addresses = [
        { type: 'home', street: `${pick(LAST)}straße ${int(1, 99)}`, city, region: '', postal, country },
      ];
    }
    if (rand() < 0.5) {
      const [organization, department, job_title] = pick(COMPANIES);
      Object.assign(contact, { organization, department, job_title });
    }
    if (rand() < 0.3) contact.food_preference = pick(FOODS);
    if (rand() < 0.4) contact.how_we_met = pick(MET);
    if (rand() < 0.1) contact.anniversary = `${int(2005, 2022)}-${pad(int(1, 12))}-${pad(int(1, 28))}`;
    if (rand() < 0.15) contact.urls = [{ type: 'website', value: `https://${slug}.example.org` }];
    result.push(contact);
  }
  return result;
}

const json = { 'Content-Type': 'application/json' };
const call = (method, path, body, headers = {}) =>
  fetch(`${base}${path}`, { method, headers: { ...json, ...headers }, body: body && JSON.stringify(body) });
const post = (path, body, headers) => call('POST', path, body, headers);

async function main() {
  const health = await fetch(`${base.replace(/\/api\/v1$/, '')}/health`).catch(() => null);
  if (!health?.ok) throw new Error(`No backend at ${base}. Start it with "make dev" first.`);

  const reg = await post('/register', USER);
  if (reg.ok) console.log(`Registered user ${USER.username}`);
  else if (reg.status === 409) console.log(`User ${USER.username} exists`);
  else throw new Error(`Register failed: ${reg.status} ${await reg.text()} (is DISABLE_REGISTRATION set?)`);

  const login = await post('/login', { identifier: USER.username, password: USER.password });
  if (!login.ok) throw new Error(`Login failed: ${login.status} ${await login.text()}`);
  const cookie = login.headers
    .getSetCookie()
    .map((c) => c.split(';')[0])
    .join('; ');
  const auth = { Cookie: cookie };

  let failed = 0;
  const send = async (label, path, body) => {
    const res = await post(path, body, auth);
    if (!res.ok) {
      failed++;
      console.log(`  Failed: ${label}: ${res.status} ${await res.text()}`);
    }
    return res.ok ? res.json() : null;
  };

  const created = [];
  for (const contact of [...handcrafted, ...generated(51)]) {
    const name = `${contact.firstname} ${contact.lastname}`;
    const found = await fetch(`${base}/contacts?search=${encodeURIComponent(name)}&limit=5`, { headers: auth });
    if (!found.ok) throw new Error(`Contact search failed: ${found.status} ${await found.text()}`);
    const list = (await found.json()).contacts ?? [];
    if (list.some((c) => c.firstname === contact.firstname && c.lastname === contact.lastname)) {
      console.log(`  Exists: ${name}`);
      continue;
    }
    const body = await send(name, '/contacts', contact);
    if (body) {
      created.push({ id: body.contact.ID, name });
      console.log(`  Created: ${name}`);
    }
  }

  for (const { id, name } of created) {
    for (let i = int(0, 4); i > 0; i--) {
      await send(`note for ${name}`, `/contacts/${id}/notes`, {
        content: pick(NOTES),
        date: daysFromNow(-int(1, 700)),
      });
    }
    if (rand() < 0.7) {
      const [recurringMessage, recurrence] = pick(REMINDERS);
      await send(`reminder for ${name}`, `/contacts/${id}/reminders`, {
        message: `${recurringMessage} (${name})`,
        remind_at: daysFromNow(int(-10, 90)),
        recurrence,
        by_mail: rand() < 0.3,
        reoccur_from_completion: rand() < 0.3,
        contact_id: id,
      });
    }
    for (let i = int(0, 2); i > 0; i--) {
      const [title, location, description] = pick(ACTIVITIES);
      const others = some(created, int(0, 2)).map((c) => c.id);
      await send(`activity for ${name}`, '/activities', {
        title,
        location,
        description,
        date: daysFromNow(-int(1, 900), 18),
        contact_ids: [...new Set([id, ...others])],
      });
    }
  }

  if (created.length > 0) {
    const start = -int(30, 300);
    await send('activity over several days', '/activities', {
      title: 'Weekend trip',
      location: 'Lake Constance',
      description: 'Two nights in a guesthouse by the water',
      date: daysFromNow(start, 0),
      end_date: daysFromNow(start + 2, 0),
      contact_ids: some(created, 2).map((c) => c.id),
    });
  }

  for (const [i, a] of created.entries()) {
    const b = created[(i * 7 + 3) % created.length];
    if (!b || b.id === a.id || rand() < 0.5) continue;
    await send(`relationship ${a.name}`, `/contacts/${a.id}/relationships`, {
      name: b.name,
      type: pick(REL_TYPES),
      related_contact_id: b.id,
    });
  }
  if (created.length > 0) {
    await send('relationship without contact', `/contacts/${created[0].id}/relationships`, {
      name: 'Old Neighbour',
      type: 'Neighbor',
      birthday: isoDate(new Date(Date.UTC(1950, 4, 5))),
    });
    for (const content of NOTES.slice(0, 3))
      await send('unassigned note', '/notes', { content, date: daysFromNow(-int(1, 60)) });
  }

  if (failed > 0) throw new Error(`${failed} requests failed`);
  console.log(`Created ${created.length} contacts. Log in as ${USER.username} / ${USER.password}`);
}

main().catch((e) => {
  console.error(e.message);
  process.exit(1);
});

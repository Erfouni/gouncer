# @gopherium/react-auth

Session authentication for React apps backed by gouncer's authkit.

## Entry points

- `@gopherium/react-auth` is the headless core: the auth API client,
  `useSession`, `useLogout`, `sessionQueryKey`, the slot-based
  `AuthGate`, `createAuthQueryClient`, and `isSessionRevoked`.
- `@gopherium/react-auth/admin` is the user administration API client.
- `@gopherium/react-auth/wpds` renders the WordPress Design System UI:
  `LoginScreen` (brand prop), `AccountPanel`, `UsersScreen`,
  `NewUserScreen`, `usersMenuItem`, `SetupCodeStep`, which asks for
  the one-time setup code as Jenkins' "Unlock Jenkins" does, and
  `SetupAccountStep`, the account rows of WordPress's installer with
  its password strength meter. Import
  `@gopherium/react-auth/wpds/style.css` alongside it.
- `@gopherium/react-auth/testing` is the msw harness: `server`,
  `installTestEnvironment`, `seedSession`, `defaultUser`, and canned
  handlers for the auth endpoints.

## Mount

```tsx
const client = createAuthQueryClient()

<QueryClientProvider client={client}>
  <AuthGate loginScreen={(onLogin) => <LoginScreen brand="MyApp" onLogin={onLogin} />}>
    <App />
  </AuthGate>
</QueryClientProvider>
```

## Setup wizard rows

`SetupAccountStep` shows the wizard's own rows in `before`, above
Name, and in `after`, below Your Email. They line up with the step's
rows only when they use the same hooks:

- `className="gopherium-setup__row"` on each row.
- `render={<div />}` on `Field.Root`, so the row adds no gap of its
  own.
- `className="gopherium-setup__label"` on the row's label.
- `className="gopherium-setup__checkbox"` on a `CheckboxControl`.
- `className="gopherium-setup__help"` on a help line under the field.

A checkbox row is a plain `<div className="gopherium-setup__row">`
with a `Field.VisualLabel`, as the step's Confirm Password row is.

```tsx
<SetupAccountStep
  code={code}
  submitLabel="Install MyApp"
  onCreated={onCreated}
  before={
    <Field.Root className="gopherium-setup__row" render={<div />}>
      <Field.Label className="gopherium-setup__label">Site Title</Field.Label>
      <div>
        <Input value={title} onChange={(event) => setTitle(event.target.value)} />
      </div>
    </Field.Root>
  }
/>
```

## Consumer requirements

- `react` and `@tanstack/react-query` are peer dependencies; the app
  must own exactly one copy of each. With workspace or linked installs,
  add both to your bundler's dedupe list.
- The client calls same-origin `/api/auth/*` and `/api/users` paths. In
  development, proxy `/api` to your backend so the `__Host-` session
  cookie is first-party.
- `/wpds` needs the `@wordpress/ui` and `@wordpress/icons` peers.
- The account step loads zxcvbn, about 400 kB gzipped, as its own chunk
  the first time it renders. Vite reports that chunk as larger than
  500 kB.

## License

Apache-2.0. Copyright © 2026 Manuel 'SirLouen' Camargo.

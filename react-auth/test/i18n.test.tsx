// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { setLocaleData } from '@wordpress/i18n'
import { afterEach, expect, test, vi } from 'vitest'

import { DOMAIN } from '../src/domain'
import { seedSession } from '../src/testing'
import { AccountPanel, LoginScreen, SetupAccountStep, SetupCodeStep, usersNavItem } from '../src/wpds'

vi.mock('../src/wpds/strength', async (importOriginal) => ({
	...(await importOriginal<typeof import('../src/wpds/strength')>()),
	loadMeter: async () => () => 4,
}))

setLocaleData(
	{
		'admin section\u0004Users': ['Usuarios'],
		'field label\u0004Email': ['Correo'],
		Password: ['Contrasena'],
		'Log in': ['Entrar'],
		'Log out': ['Salir'],
		'Unlock %s': ['Desbloquear %s'],
		'Setup code': ['Codigo'],
		'Your Email': ['Tu correo electrónico'],
		'password strength\u0004Strong': ['Fuerte'],
	},
	DOMAIN,
)

afterEach(() => {
	vi.unstubAllGlobals()
})

/**
 * Renders a tree needing a query client.
 * @param tree - The element to render.
 */
function renderWith(tree: React.ReactNode) {
	const client = new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: Infinity },
			mutations: { retry: false },
		},
	})
	seedSession(client)
	render(<QueryClientProvider client={client}>{tree}</QueryClientProvider>)
}

test('names the brick after itself and not after any product', () => {
	expect(DOMAIN).toBe('gopherium-react-auth')
})

test('the users nav label follows the catalogue the consumer loaded', () => {
	expect(usersNavItem.label).toBe('Usuarios')
})

test('the login screen follows the catalogue the consumer loaded', async () => {
	renderWith(<LoginScreen brand="Testbed" onLogin={vi.fn()} />)

	expect(await screen.findByLabelText('Correo')).toBeInTheDocument()
	expect(screen.getByLabelText('Contrasena')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Entrar' })).toBeInTheDocument()
})

test('leaves the brand the consumer supplied untranslated', async () => {
	renderWith(<LoginScreen brand="Testbed" onLogin={vi.fn()} />)

	expect(await screen.findByText('Testbed')).toBeInTheDocument()
})

test('the account panel follows the catalogue the consumer loaded', async () => {
	renderWith(<AccountPanel className="testbed-account" />)

	expect(await screen.findByRole('button', { name: 'Salir' })).toBeInTheDocument()
})

test('the code step follows the catalogue the consumer loaded', () => {
	vi.stubGlobal('matchMedia', (query: string) => ({ matches: false, media: query }))
	renderWith(<SetupCodeStep brand="Testbed" fileHint="state/setup-code.txt" onUnlocked={vi.fn()} />)

	expect(screen.getByLabelText('Codigo')).toBeInTheDocument()
	expect(screen.getByRole('heading', { level: 1, name: 'Desbloquear Testbed' })).toBeInTheDocument()
})

test('the account step follows the catalogue the consumer loaded', async () => {
	renderWith(<SetupAccountStep code="K7QW3MZTB2XQ5LJ4N6RAHYCDEF" submitLabel="Testbed" onCreated={vi.fn()} />)

	expect(await screen.findByLabelText('Tu correo electrónico')).toBeInTheDocument()
	expect(screen.getByLabelText('Contrasena')).toBeInTheDocument()
})

test('the meter follows the catalogue the consumer loaded', async () => {
	renderWith(<SetupAccountStep code="K7QW3MZTB2XQ5LJ4N6RAHYCDEF" submitLabel="Testbed" onCreated={vi.fn()} />)

	expect(await screen.findByText('Fuerte')).toBeInTheDocument()
})

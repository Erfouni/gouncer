// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Link } from '@wordpress/ui'
import { http } from 'msw'
import { expect, test, vi } from 'vitest'

import {
	defaultUser,
	loginFailure,
	loginInvalid,
	loginOk,
	loginRateLimited,
	server,
} from '../src/testing'
import { LoginScreen } from '../src/wpds'
import { busyClasses } from './busy'

function renderLogin() {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	const onLogin = vi.fn()
	render(
		<QueryClientProvider client={client}>
			<LoginScreen brand="Testbed" onLogin={onLogin} />
		</QueryClientProvider>,
	)
	return onLogin
}

async function submitCredentials(email: string, password: string) {
	await userEvent.type(screen.getByLabelText('Email'), email)
	await userEvent.type(screen.getByLabelText('Password'), password)
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))
}

test('shows the log in button busy and still refuses a second submit', async () => {
	const busy = busyClasses()
	expect(busy.length).toBeGreaterThan(0)
	server.use(http.post('/api/auth/login', () => new Promise(() => {})))
	renderLogin()

	await submitCredentials('maria@example.com', 'correct horse battery')

	const submit = screen.getByRole('button', { name: 'Log in' })
	await waitFor(() => {
		for (const token of busy) {
			expect([...submit.classList]).toContain(token)
		}
	})
	expect(submit).toHaveAttribute('aria-disabled', 'true')
})

test('shows the login form under the given brand', () => {
	renderLogin()

	expect(screen.getByRole('heading', { name: 'Testbed' })).toBeInTheDocument()
	expect(screen.getByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByLabelText('Password')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Log in' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('reports the user after a successful login', async () => {
	server.use(loginOk())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'correct horse battery')

	expect(onLogin).toHaveBeenCalledWith(defaultUser)
})

test('shows an error on rejected credentials', async () => {
	server.use(loginInvalid())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'wrong password!')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Invalid email or password.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})

test('shows a rate-limit message when too many attempts are made', async () => {
	server.use(loginRateLimited())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Too many attempts. Please wait a minute and try again.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})

test('shows a generic error when the backend fails', async () => {
	server.use(loginFailure())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Login failed, please try again.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})

test('hints browsers to offer the saved login', async () => {
	renderLogin()

	expect(await screen.findByLabelText('Email')).toHaveAttribute('autocomplete', 'username')
	expect(screen.getByLabelText('Password')).toHaveAttribute(
		'autocomplete',
		'current-password',
	)
})

function renderLoginOffering(onForgotPassword: () => void) {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	render(
		<QueryClientProvider client={client}>
			<LoginScreen brand="Testbed" onLogin={vi.fn()} onForgotPassword={onForgotPassword} />
		</QueryClientProvider>,
	)
}

test('offers the lost password way only when one is provided', async () => {
	const onForgotPassword = vi.fn()
	renderLoginOffering(onForgotPassword)

	await userEvent.click(await screen.findByRole('button', { name: 'Lost your password?' }))

	await waitFor(() => expect(onForgotPassword).toHaveBeenCalled())
})

test('puts the lost password way under the login card, as WordPress does', async () => {
	renderLoginOffering(vi.fn())

	const lost = await screen.findByRole('button', { name: 'Lost your password?' })
	const nav = lost.closest('.gopherium-login__nav')

	expect(lost.closest('.gopherium-login__card')).toBeNull()
	expect(nav?.previousElementSibling).toHaveClass('gopherium-login__card')
	expect(nav?.parentElement).toHaveClass('gopherium-login__column')
})

function neutralLinkClasses() {
	const { container, unmount } = render(
		<Link tone="neutral" href="/">
			link
		</Link>,
	)
	const tokens = [...(container.querySelector('a') as Element).classList]
	unmount()
	return tokens
}

test('draws the lost password way as a link, as WordPress does', async () => {
	const tokens = neutralLinkClasses()
	expect(tokens.length).toBeGreaterThan(0)
	renderLoginOffering(vi.fn())

	const lost = await screen.findByRole('button', { name: 'Lost your password?' })

	expect([...lost.classList]).toEqual(expect.arrayContaining(tokens))
})

test('renders no lost password way when none is provided', async () => {
	renderLogin()

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(
		screen.queryByRole('button', { name: 'Lost your password?' }),
	).not.toBeInTheDocument()
	expect(document.querySelector('.gopherium-login__nav')).toBeNull()
})

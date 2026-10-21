// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderToString } from 'react-dom/server'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { configureAuthTransport, resetAuthTransport } from '../src'
import { HttpResponse, http, server } from '../src/testing'
import { SetupCodeStep } from '../src/wpds'
import { busyClasses } from './busy'

const intro =
	'To ensure this site is securely set up by its owner, a setup code has been written to a file on the server.'
const copyLine = 'Please copy the setup code from the file and paste it below.'
const invalidSentence = 'Your setup code appears to be invalid. Please copy it again from the setup code file.'
const expiredSentence = 'Your setup code has expired. A new one is waiting in the setup code file.'
const fileSentence = 'The setup code file cannot be used. Please check it and try again.'
const closedSentence = 'You appear to have already set up this site.'
const startingSentence = 'The site is starting. Please wait a moment and try again.'
const throttledSentence = 'Too many attempts. Please try again later.'
const requestSentence = 'The request failed, please try again.'

const hintPath = 'myapp-data/state/setup-code.txt'
const hintText = `In File Manager, open ${hintPath}.`
const fileHint = (
	<>
		In File Manager, open <code>{hintPath}</code>.
	</>
)

const pastedCode = 'K7QW3MZTB2XQ5LJ4N6RAHYCDEF'
const typedCode = ' k7qw-3mzt b2xq5lj4n6rahycdef '

const invalidBody = { error: 'setup code invalid', code: 'setup_code_invalid' }
const closedBody = { error: 'setup already done', code: 'setup_done' }

function pointerQuery(coarse: boolean) {
	return (query: string) => ({ matches: coarse && query === '(pointer: coarse)', media: query })
}

beforeEach(() => {
	vi.stubGlobal('matchMedia', pointerQuery(false))
})

afterEach(() => {
	vi.unstubAllGlobals()
	resetAuthTransport()
})

function renderStep(
	props: {
		onUnlocked?: (code: string) => void | Promise<void>
		onClosed?: () => void
		refused?: 'invalid' | 'expired' | 'file'
	} = {},
) {
	const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
	const onUnlocked = props.onUnlocked ?? vi.fn()
	const view = render(
		<QueryClientProvider client={client}>
			<SetupCodeStep
				brand="Testbed"
				fileHint={fileHint}
				onUnlocked={onUnlocked}
				onClosed={props.onClosed}
				refused={props.refused}
			/>
		</QueryClientProvider>,
	)
	return { ...view, onUnlocked }
}

function codeAnswer(status: number, body: object | string | null) {
	return http.post('/api/auth/setup/code', () =>
		typeof body === 'object' && body !== null
			? HttpResponse.json(body, { status })
			: new HttpResponse(body, { status }),
	)
}

async function submitCode(code: string) {
	await userEvent.type(screen.getByLabelText('Setup code'), code)
	await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
}

function expectInOrder(...nodes: Element[]) {
	for (let index = 1; index < nodes.length; index += 1) {
		const following = nodes[index - 1].compareDocumentPosition(nodes[index]) & Node.DOCUMENT_POSITION_FOLLOWING
		expect(following, `node ${index} follows node ${index - 1}`).not.toBe(0)
	}
}

test('shows the unlock heading under the given brand', () => {
	renderStep()

	expect(screen.getByRole('heading', { level: 1, name: 'Unlock Testbed' })).toBeInTheDocument()
})

test("follows Jenkins' order from the heading to Continue", () => {
	renderStep()

	expectInOrder(
		screen.getByRole('heading', { level: 1 }),
		screen.getByText(intro),
		screen.getByText(hintPath),
		screen.getByText(copyLine),
		screen.getByText('Setup code'),
		screen.getByLabelText('Setup code'),
		screen.getByRole('button', { name: 'Continue' }),
	)
})

test("draws the row label as WordPress's th label", () => {
	renderStep()

	expect(screen.getByText('Setup code')).toHaveClass('gopherium-setup__label')
})

test('lays the row out as the plain grid row, with no gap of its own', () => {
	const { container } = renderStep()

	const row = container.querySelector('.gopherium-setup__row')
	expect(row).not.toBeNull()
	expect(row).not.toHaveAttribute('style')
})

test("gives the intro and the copy line WordPress's paragraph margins", () => {
	renderStep()

	expect(screen.getByText(intro)).toHaveClass('gopherium-setup__text')
	expect(screen.getByText(copyLine)).toHaveClass('gopherium-setup__text')
})

test("puts a refusal above the field in WordPress's message box", async () => {
	server.use(codeAnswer(401, invalidBody))
	renderStep()

	await submitCode(pastedCode)

	const alert = await screen.findByRole('alert')
	expectInOrder(screen.getByText(copyLine), alert, screen.getByLabelText('Setup code'))
	expect(alert).toHaveClass('gopherium-setup__message')
})

test("describes the field with the application's hint", async () => {
	server.use(codeAnswer(401, invalidBody))
	renderStep()
	const field = screen.getByLabelText('Setup code')

	expect(field).toHaveAccessibleDescription(hintText)

	await submitCode(pastedCode)

	await screen.findByRole('alert')
	expect(field).toHaveAccessibleDescription(`${invalidSentence} ${hintText}`)
})

test('keeps the code readable and out of autofill', () => {
	renderStep()

	const field = screen.getByLabelText('Setup code')
	expect(field).toHaveAttribute('type', 'text')
	expect(field).toHaveAttribute('autocomplete', 'off')
	expect(field).toHaveAttribute('spellcheck', 'false')
})

test('takes focus when it appears', () => {
	renderStep()

	expect(screen.getByLabelText('Setup code')).toHaveFocus()
})

test('shows the form with a disabled Continue until a code is typed', async () => {
	renderStep()
	const submit = screen.getByRole('button', { name: 'Continue' })

	expect(submit).toHaveAttribute('aria-disabled', 'true')

	await userEvent.type(screen.getByLabelText('Setup code'), '   ')
	expect(submit).toHaveAttribute('aria-disabled', 'true')

	await userEvent.type(screen.getByLabelText('Setup code'), 'K')
	expect(submit).not.toHaveAttribute('aria-disabled', 'true')
})

test('locks Continue while the check is in flight', async () => {
	const busy = busyClasses()
	expect(busy.length).toBeGreaterThan(0)
	let posts = 0
	server.use(
		http.post('/api/auth/setup/code', () => {
			posts += 1
			return new Promise<never>(() => {})
		}),
	)
	renderStep()

	await submitCode(pastedCode)

	const submit = screen.getByRole('button', { name: 'Continue' })
	await waitFor(() => {
		for (const token of busy) {
			expect([...submit.classList]).toContain(token)
		}
	})
	expect(submit).toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(submit)
	expect(posts).toBe(1)
})

function codeForm() {
	return screen.getByRole('button', { name: 'Continue' }).closest('form') as HTMLFormElement
}

function waitForPendingWork() {
	return act(() => new Promise((resolve) => setTimeout(resolve)))
}

test('checks nothing when the form itself is submitted with a blank code', async () => {
	const check = vi.fn(async () => {})
	configureAuthTransport({ checkSetupCode: check })
	const { onUnlocked } = renderStep()

	fireEvent.submit(codeForm())
	await userEvent.type(screen.getByLabelText('Setup code'), '   ')
	fireEvent.submit(codeForm())
	await waitForPendingWork()

	expect(check).not.toHaveBeenCalled()
	expect(onUnlocked).not.toHaveBeenCalled()
})

test('checks the code once when the form itself is submitted again while the check is in flight', async () => {
	const check = vi.fn(() => new Promise<never>(() => {}))
	configureAuthTransport({ checkSetupCode: check })
	renderStep()
	await submitCode(pastedCode)
	const submit = screen.getByRole('button', { name: 'Continue' })
	await waitFor(() => expect(submit).toHaveAttribute('aria-disabled', 'true'))

	fireEvent.submit(codeForm())
	await waitForPendingWork()

	expect(check).toHaveBeenCalledOnce()
})

test('posts the code exactly as typed', async () => {
	let body: unknown
	server.use(
		http.post('/api/auth/setup/code', async ({ request }) => {
			body = await request.json()
			return new HttpResponse(null, { status: 204 })
		}),
	)
	const { onUnlocked } = renderStep()

	await submitCode(typedCode)

	await waitFor(() => expect(onUnlocked).toHaveBeenCalled())
	expect(body).toEqual({ code: typedCode })
})

test('hands the code upward once the site accepts it', async () => {
	server.use(codeAnswer(204, null))
	const { onUnlocked } = renderStep()

	await submitCode(typedCode)

	await waitFor(() => expect(onUnlocked).toHaveBeenCalledOnce())
	expect(onUnlocked).toHaveBeenCalledWith(typedCode)
})

test('tells each refusal in its own words and keeps the code', async () => {
	const refusals = [
		{ status: 401, body: invalidBody, server: 'setup code invalid', sentence: invalidSentence },
		{
			status: 401,
			body: { error: 'setup code expired', code: 'setup_code_expired' },
			server: 'setup code expired',
			sentence: expiredSentence,
		},
		{
			status: 500,
			body: { error: 'setup code file unusable', code: 'setup_code_file' },
			server: 'setup code file unusable',
			sentence: fileSentence,
		},
		{ status: 409, body: closedBody, server: 'setup already done', sentence: closedSentence },
		{ status: 503, body: 'Service Unavailable', server: 'Service Unavailable', sentence: startingSentence },
		{
			status: 429,
			body: { error: 'too many login attempts, try again later', code: 'login_rate_limited' },
			server: 'too many login attempts',
			sentence: throttledSentence,
		},
	]
	const { container, onUnlocked } = renderStep()
	const field = screen.getByLabelText('Setup code')
	await userEvent.type(field, pastedCode)

	for (const row of refusals) {
		server.use(codeAnswer(row.status, row.body))
		await userEvent.click(screen.getByRole('button', { name: 'Continue' }))

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(row.sentence))
		expect(field).toHaveValue(pastedCode)
		expect(container).not.toHaveTextContent(row.server)
	}
	expect(onUnlocked).not.toHaveBeenCalled()
})

test('reports a site that is already set up', async () => {
	server.use(codeAnswer(409, closedBody))
	const onClosed = vi.fn()
	renderStep({ onClosed })

	await submitCode(pastedCode)

	await waitFor(() => expect(onClosed).toHaveBeenCalledOnce())
	expect(screen.getByRole('alert')).toHaveTextContent(closedSentence)
})

test('shows why the wizard came back', async () => {
	server.use(http.post('/api/auth/setup/code', () => new Promise<never>(() => {})))
	renderStep({ refused: 'expired' })
	const field = screen.getByLabelText('Setup code')

	expect(screen.getByRole('alert')).toHaveTextContent(expiredSentence)
	expect(field).toHaveAccessibleDescription(`${expiredSentence} ${hintText}`)

	await submitCode(pastedCode)

	await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
	expect(field).toHaveAccessibleDescription(hintText)
})

test('checks again when handing the code onward fails', async () => {
	let posts = 0
	server.use(
		http.post('/api/auth/setup/code', () => {
			posts += 1
			return new HttpResponse(null, { status: 204 })
		}),
	)
	const onUnlocked = vi.fn().mockRejectedValueOnce(new Error('the wizard could not move on'))
	renderStep({ onUnlocked })

	await submitCode(pastedCode)

	expect(await screen.findByRole('alert')).toHaveTextContent(requestSentence)
	await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
	await waitFor(() => expect(onUnlocked).toHaveBeenCalledTimes(2))
	expect(posts).toBe(2)
})

test('shows a generic message when the check fails', async () => {
	server.use(codeAnswer(500, { error: 'internal error', code: 'internal' }))
	const { container } = renderStep()

	await submitCode(pastedCode)

	expect(await screen.findByRole('alert')).toHaveTextContent(requestSentence)
	expect(container).not.toHaveTextContent('internal error')
})

test('meets every refusal without the optional callback', async () => {
	const stray = vi.fn()
	process.on('unhandledRejection', stray)
	server.use(codeAnswer(409, closedBody))
	renderStep()

	await submitCode(pastedCode)

	expect(await screen.findByRole('alert')).toHaveTextContent(closedSentence)
	await new Promise((resolve) => setTimeout(resolve))
	process.off('unhandledRejection', stray)
	expect(stray).not.toHaveBeenCalled()
})

test('keeps the sentence that sent the wizard back away once the site accepts the code', async () => {
	server.use(codeAnswer(204, null))
	const { onUnlocked } = renderStep({ refused: 'expired' })

	await submitCode(pastedCode)

	await waitFor(() => expect(onUnlocked).toHaveBeenCalledOnce())
	expect(screen.queryByRole('alert')).toBeNull()
})

test('names no product', () => {
	const { container } = renderStep({ refused: 'invalid' })

	expect(container).not.toHaveTextContent(/WordPress|Jenkins/)
})

test('leaves focus alone on a touch screen, as WordPress does on phones', () => {
	vi.stubGlobal('matchMedia', pointerQuery(true))
	renderStep()

	expect(screen.getByLabelText('Setup code')).not.toHaveFocus()
})

test('takes focus where the window cannot tell the pointer', () => {
	vi.stubGlobal('matchMedia', undefined)
	renderStep()

	expect(screen.getByLabelText('Setup code')).toHaveFocus()
})

test('takes focus only when it appears, never again when a refusal comes back', async () => {
	server.use(codeAnswer(401, invalidBody))
	renderStep()

	await submitCode(pastedCode)

	await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(invalidSentence))
	expect(screen.getByRole('button', { name: 'Continue' })).toHaveFocus()
})

test('renders on a server, where there is no window, with no autofocus for a phone to obey', () => {
	vi.stubGlobal('window', undefined)
	Reflect.deleteProperty(globalThis, 'window')
	expect('window' in globalThis).toBe(false)

	const html = renderToString(
		<QueryClientProvider client={new QueryClient()}>
			<SetupCodeStep brand="Testbed" fileHint={fileHint} onUnlocked={vi.fn()} />
		</QueryClientProvider>,
	)
	vi.unstubAllGlobals()

	expect(html).toContain('Unlock Testbed')
	expect(html).toContain(hintPath)
	expect(html).not.toMatch(/autofocus/i)
})

// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { configureAuthTransport, resetAuthTransport } from '../src'
import {
	HttpResponse,
	createdAccount,
	firstAccountClosed,
	firstAccountCodeRefused,
	firstAccountOk,
	firstAccountRateLimited,
	firstAccountStarting,
	http,
	server,
	setupStatus,
} from '../src/testing'
import { SetupAccountStep } from '../src/wpds'
import type { Meter } from '../src/wpds/strength'
import { busyClasses } from './busy'

const meter = vi.hoisted(() => ({ load: vi.fn() }))

vi.mock('../src/wpds/strength', async (importOriginal) => ({
	...(await importOriginal<typeof import('../src/wpds/strength')>()),
	loadMeter: meter.load,
}))

const setupCode = 'K7QW3MZTB2XQ5LJ4N6RAHYCDEF'
const emailHelp = 'Double-check your email address before continuing.'
const nameAndEmailWords = ['Maria', 'Perez', 'maria', 'example']

beforeEach(() => {
	meter.load.mockReset()
	meter.load.mockReturnValue(new Promise(() => {}))
})

afterEach(() => {
	resetAuthTransport()
})

function renderStep(props: Partial<ComponentProps<typeof SetupAccountStep>> = {}) {
	const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
	const onCreated = props.onCreated ?? vi.fn()
	const view = render(
		<QueryClientProvider client={client}>
			<SetupAccountStep code={setupCode} submitLabel="Install Testbed" {...props} onCreated={onCreated} />
		</QueryClientProvider>,
	)
	return { ...view, onCreated }
}

function expectInOrder(...nodes: Element[]) {
	for (let index = 1; index < nodes.length; index += 1) {
		const following = nodes[index - 1].compareDocumentPosition(nodes[index]) & Node.DOCUMENT_POSITION_FOLLOWING
		expect(following, `node ${index} follows node ${index - 1}`).not.toBe(0)
	}
}

function describedBy(field: Element): HTMLElement[] {
	const ids = field.getAttribute('aria-describedby')?.split(' ') ?? []
	return ids.map((id) => document.getElementById(id) as HTMLElement)
}

function holdAccounts() {
	let posts = 0
	server.use(
		http.post('/api/auth/setup', () => {
			posts += 1
			return new Promise<never>(() => {})
		}),
	)
	return () => posts
}

async function fillAccount(name = 'Maria Perez', email = 'maria@example.com') {
	await userEvent.type(screen.getByLabelText('Name'), name)
	await userEvent.type(screen.getByLabelText('Your Email'), email)
}

function holdMeter() {
	let land: (loaded: Meter) => void = () => {}
	let fail: (error: Error) => void = () => {}
	meter.load.mockReturnValue(
		new Promise<Meter>((resolve, reject) => {
			land = resolve
			fail = reject
		}),
	)
	return {
		land: (loaded: Meter) => act(async () => land(loaded)),
		fail: () => act(async () => fail(new Error('the meter chunk did not load'))),
	}
}

function fakeMeter(scores: Record<string, number> = {}, otherwise = 4) {
	return vi.fn<Meter>((password) => scores[password] ?? otherwise)
}

function typePassword(value: string) {
	fireEvent.change(screen.getByLabelText('Password'), { target: { value } })
}

function meterBox() {
	return describedBy(screen.getByLabelText('Password'))[0]
}

function submitButton() {
	return screen.getByRole('button', { name: 'Install Testbed' })
}

function confirmBox() {
	return screen.queryByRole('checkbox', { name: 'Confirm use of weak password' })
}

function expectSubmitOn(on: boolean) {
	if (on) {
		expect(submitButton()).not.toHaveAttribute('aria-disabled', 'true')
	} else {
		expect(submitButton()).toHaveAttribute('aria-disabled', 'true')
	}
}

test("lays out Name, Password and Your Email in WordPress's order between the wizard's rows", () => {
	renderStep({ before: <p>Site Title row</p>, after: <p>Search engine visibility row</p> })

	expectInOrder(
		screen.getByText('Site Title row'),
		screen.getByText('Name'),
		screen.getByLabelText('Name'),
		screen.getByText('Password'),
		screen.getByLabelText('Password'),
		screen.getByText('Your Email'),
		screen.getByLabelText('Your Email'),
		screen.getByText('Search engine visibility row'),
		screen.getByRole('button', { name: 'Install Testbed' }),
	)
})

test('describes the email field with the line asking to check it', () => {
	renderStep()

	expect(screen.getByLabelText('Your Email')).toHaveAccessibleDescription(emailHelp)
})

test('describes the password with the line asking to store it', () => {
	renderStep()

	const field = screen.getByLabelText('Password')
	expect(field).toHaveAccessibleDescription(expect.stringContaining('Important:'))
	expect(field).toHaveAccessibleDescription(expect.stringMatching(/You will need this password to log.in\./))
})

test('keeps the no-break space WordPress puts in log in', () => {
	renderStep()

	const help = screen.getByText(/You will need this password/)
	expect(help.textContent).toContain('log\u00a0in')
	expect(help.textContent).not.toContain('log in')
})

test('hints each field for the browser and password managers', () => {
	renderStep()

	const name = screen.getByLabelText('Name')
	const password = screen.getByLabelText('Password')
	const email = screen.getByLabelText('Your Email')
	expect(name).toHaveAttribute('autocomplete', 'name')
	expect(name).toHaveAttribute('type', 'text')
	expect(password).toHaveAttribute('autocomplete', 'new-password')
	expect(password).toHaveAttribute('spellcheck', 'false')
	expect(email).toHaveAttribute('autocomplete', 'username')
	expect(email).toHaveAttribute('type', 'email')
})

test("names the submit with the wizard's own words", () => {
	renderStep({ submitLabel: 'Set up Testbed' })

	expect(screen.getByRole('button', { name: 'Set up Testbed' })).toHaveAttribute('type', 'submit')
})

test('names no product', () => {
	const { container } = renderStep()

	expect(container).not.toHaveTextContent(/WordPress/)
})

test('wires the meter box before the help', () => {
	renderStep()

	const [box, help] = describedBy(screen.getByLabelText('Password'))
	expect(box).toHaveAttribute('aria-live', 'polite')
	expect(help).toHaveTextContent(/^Important:/)
})

test("starts with a password WordPress's installer would make, shown in clear", () => {
	renderStep()

	const field = screen.getByLabelText('Password') as HTMLInputElement
	expect(field).toHaveAttribute('type', 'text')
	expect(field.value).toMatch(/^[A-Za-z0-9!@#$%^&*()]{18}$/)
})

test('makes the starting password as long as the wizard asks', () => {
	renderStep({ generatedPasswordLength: 24 })

	expect((screen.getByLabelText('Password') as HTMLInputElement).value).toMatch(/^[A-Za-z0-9!@#$%^&*()]{24}$/)
})

test('hides and shows the password with its toggle', async () => {
	renderStep()
	const field = screen.getByLabelText('Password')

	await userEvent.click(screen.getByRole('button', { name: 'Hide password' }))

	expect(field).toHaveAttribute('type', 'password')
	expect(screen.getByRole('button', { name: 'Show password' })).toHaveTextContent(/^Show$/)

	await userEvent.click(screen.getByRole('button', { name: 'Show password' }))

	expect(field).toHaveAttribute('type', 'text')
	expect(screen.getByRole('button', { name: 'Hide password' })).toHaveTextContent(/^Hide$/)
})

test('masks the password when the form is sent, so a password manager offers to save it', async () => {
	holdAccounts()
	renderStep()
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Install Testbed' }))

	expect(screen.getByLabelText('Password')).toHaveAttribute('type', 'password')
	expect(screen.getByRole('button', { name: 'Show password' })).toHaveTextContent(/^Show$/)
})

test('warns while Caps Lock is on', async () => {
	const user = userEvent.setup()
	const { container } = renderStep()
	const region = container.querySelector('[aria-live="assertive"]')
	expect(region).toBeEmptyDOMElement()
	await user.click(screen.getByLabelText('Password'))

	await user.keyboard('{CapsLock}k')

	expect(region).toHaveTextContent('Caps lock is on.')

	await user.keyboard('{CapsLock}k')

	expect(screen.queryByText('Caps lock is on.')).toBeNull()
})

test('reads Caps Lock only from printable keys, as WordPress does', async () => {
	const user = userEvent.setup()
	renderStep()
	await user.click(screen.getByLabelText('Password'))
	await user.keyboard('{CapsLock}k')

	await user.keyboard('{CapsLock}{ArrowLeft}{Control>}k{/Control}{Alt>}k{/Alt}{Meta>}k{/Meta}')

	expect(screen.getByText('Caps lock is on.')).toBeInTheDocument()
})

test('puts the Caps Lock line after the meter', async () => {
	const user = userEvent.setup()
	renderStep()
	const field = screen.getByLabelText('Password')
	await user.click(field)

	await user.keyboard('{CapsLock}k')

	const [box] = describedBy(field)
	expectInOrder(field, box, screen.getByText('Caps lock is on.'))
})

test('shows a blank meter and an open submit until the meter has loaded', async () => {
	holdMeter()
	renderStep()
	await fillAccount()

	expect(meterBox().textContent).toBe('')
	expect(meterBox().className).not.toMatch(/--/)
	expect(screen.queryByText('Confirm Password')).toBeNull()
	expectSubmitOn(true)
})

test('scores the generated password once the meter lands', async () => {
	const held = holdMeter()
	const scores = fakeMeter()
	renderStep()
	const generated = (screen.getByLabelText('Password') as HTMLInputElement).value

	await held.land(scores)

	expect(scores).toHaveBeenCalledOnce()
	expect(scores.mock.calls[0][0]).toBe(generated)
	expect(generated).toHaveLength(18)
	expect(meterBox()).toHaveTextContent('Strong')
})

test('marks a password typed before the meter lands as unknown', async () => {
	holdMeter()
	renderStep()
	await fillAccount()

	typePassword('perezmaria12')

	expect(meterBox()).toHaveTextContent('Password strength unknown')
	expect(confirmBox()).toBeInTheDocument()
	expectSubmitOn(false)
})

test('scores what the field holds when the meter lands', async () => {
	const held = holdMeter()
	const scores = fakeMeter({ twelvechars1: 3 })
	renderStep()
	await fillAccount()
	typePassword('twelvechars1')
	expect(meterBox()).toHaveTextContent('Password strength unknown')

	await held.land(scores)

	expect(scores).toHaveBeenLastCalledWith('twelvechars1', expect.arrayContaining(nameAndEmailWords))
	expect(meterBox()).toHaveTextContent('Medium')
})

test("shows each level in WordPress's words and tone", async () => {
	const held = holdMeter()
	renderStep()
	await held.land(fakeMeter({ abcdefghijkl: 0, password1234: 1, perezmaria12: 2, twelvechars1: 3 }))
	const levels = [
		{ password: 'abcdefghijkl', word: 'Very weak', tone: 'short' },
		{ password: 'password1234', word: 'Very weak', tone: 'short' },
		{ password: 'perezmaria12', word: 'Weak', tone: 'bad' },
		{ password: 'twelvechars1', word: 'Medium', tone: 'good' },
		{ password: 'K7#mQ2!vR9@xL4$wP8', word: 'Strong', tone: 'strong' },
	]

	for (const level of levels) {
		typePassword(level.password)

		expect(meterBox()).toHaveTextContent(level.word)
		expect(meterBox()).toHaveClass(`gopherium-setup__meter--${level.tone}`)
		expect(screen.getByLabelText('Password').closest(`.gopherium-setup__input--${level.tone}`)).not.toBeNull()
	}
})

test('keeps the submit off for a weak password until its use is confirmed', async () => {
	const held = holdMeter()
	renderStep()
	await held.land(fakeMeter({ password1234: 1 }))
	await fillAccount()

	typePassword('password1234')

	expectSubmitOn(false)
	await userEvent.click(confirmBox() as HTMLElement)
	expectSubmitOn(true)
})

test('asks no confirmation for a medium or strong password', async () => {
	const held = holdMeter()
	renderStep()
	await held.land(fakeMeter({ twelvechars1: 3 }))
	await fillAccount()

	for (const password of ['twelvechars1', 'K7#mQ2!vR9@xL4$wP8']) {
		typePassword(password)

		expect(confirmBox()).toBeNull()
		expectSubmitOn(true)
	}
})

test('keeps the confirmation while the password changes', async () => {
	const held = holdMeter()
	renderStep()
	await held.land(fakeMeter({ password1234: 1, perezmaria12: 2 }))
	await fillAccount()
	typePassword('password1234')
	await userEvent.click(confirmBox() as HTMLElement)

	typePassword('K7#mQ2!vR9@xL4$wP8')

	expect(confirmBox()).toBeNull()
	expectSubmitOn(true)

	typePassword('perezmaria12')

	expect(confirmBox()).toBeChecked()
	expectSubmitOn(true)
})

test('clears the confirmation when the password is emptied', async () => {
	const held = holdMeter()
	renderStep()
	await held.land(fakeMeter({ password1234: 1 }))
	await fillAccount()

	for (const emptied of ['', '   ']) {
		typePassword('password1234')
		await userEvent.click(confirmBox() as HTMLElement)

		typePassword(emptied)

		expect(confirmBox()).toBeNull()
		expect(meterBox().textContent).toBe('\u00a0')
		expectSubmitOn(false)

		typePassword('password1234')

		expect(confirmBox()).not.toBeChecked()
		expectSubmitOn(false)
	}
})

test('treats a meter that fails to load as unknown', async () => {
	const held = holdMeter()
	renderStep()
	await fillAccount()

	await held.fail()

	expect(meterBox().textContent).toBe('')
	expectSubmitOn(true)

	typePassword('K7#mQ2!vR9@xL4$wP8')

	expect(meterBox()).toHaveTextContent('Password strength unknown')
	expect(confirmBox()).toBeInTheDocument()
	expectSubmitOn(false)
})

test("scores against the step's own fields and the wizard's words", async () => {
	document.title = 'Testbed Setup'
	const held = holdMeter()
	const scores = fakeMeter()
	renderStep({ userInputs: ['Myapp Site Title'] })
	await held.land(scores)
	await fillAccount()

	typePassword('perezmaria12')

	const [password, words] = scores.mock.lastCall ?? []
	expect(password).toBe('perezmaria12')
	expect(words).toEqual(
		expect.arrayContaining(['Testbed', 'Setup', 'localhost', 'Maria', 'Perez', 'maria', 'example']),
	)
	expect(words).toEqual(expect.arrayContaining(['Myapp', 'Site', 'Title']))
	expect(words).not.toContain('com')
	document.title = ''
})

test('does not score again when only the name changes, as WordPress', async () => {
	const held = holdMeter()
	const scores = fakeMeter()
	renderStep()
	await held.land(scores)
	const scored = scores.mock.calls.length

	await userEvent.type(screen.getByLabelText('Name'), 'Maria Perez')

	expect(scores).toHaveBeenCalledTimes(scored)
})

test('sends only the four fields, never the confirmation', async () => {
	let body: unknown
	server.use(
		http.post('/api/auth/setup', async ({ request }) => {
			body = await request.json()
			return HttpResponse.json(createdAccount, { status: 201 })
		}),
	)
	const held = holdMeter()
	const { onCreated } = renderStep()
	await held.land(fakeMeter({ password1234: 1 }))
	await fillAccount()
	typePassword('password1234')
	await userEvent.click(confirmBox() as HTMLElement)

	await userEvent.click(submitButton())

	await waitFor(() => expect(onCreated).toHaveBeenCalled())
	expect(body).toEqual({ code: setupCode, email: 'maria@example.com', name: 'Maria Perez', password: 'password1234' })
})

test('hands the client only the four fields, never the confirmation', async () => {
	const createFirst = vi.fn(async () => createdAccount)
	configureAuthTransport({ createFirstAccount: createFirst })
	const held = holdMeter()
	const { onCreated } = renderStep()
	await held.land(fakeMeter({ password1234: 1 }))
	await fillAccount()
	typePassword('password1234')
	await userEvent.click(confirmBox() as HTMLElement)

	await userEvent.click(submitButton())

	await waitFor(() => expect(onCreated).toHaveBeenCalled())
	expect(createFirst).toHaveBeenCalledOnce()
	expect(createFirst.mock.calls[0]).toStrictEqual([
		{ code: setupCode, email: 'maria@example.com', name: 'Maria Perez', password: 'password1234' },
	])
})

test('shows the confirm row between Password and Your Email for a weak password', async () => {
	const held = holdMeter()
	renderStep()
	await held.land(fakeMeter({ password1234: 1 }))

	typePassword('password1234')

	expectInOrder(
		screen.getByLabelText('Password'),
		screen.getByText('Confirm Password'),
		confirmBox() as HTMLElement,
		screen.getByLabelText('Your Email'),
	)
})

test("starts the password's description with the meter's word", async () => {
	const held = holdMeter()
	renderStep()

	await held.land(fakeMeter())

	const field = screen.getByLabelText('Password')
	expect(field).toHaveAccessibleDescription(
		expect.stringMatching(/^Strong Important: You will need this password to log.in/),
	)
})

const hintTail = 'To make it stronger, use upper and lower case letters, numbers, and symbols like ! " ? $ % ^ & ).'
const startingSentence = 'The site is starting. Please wait a moment and try again.'
const failedSentence = 'The account could not be created, please try again.'

async function renderReady(props: Partial<ComponentProps<typeof SetupAccountStep>> = {}) {
	const held = holdMeter()
	const view = renderStep(props)
	await held.land(fakeMeter())
	return view
}

function answerAccount(status: number, body: object | null) {
	server.use(
		http.post('/api/auth/setup', () =>
			body === null ? new HttpResponse(null, { status }) : HttpResponse.json(body, { status }),
		),
	)
}

function refusal(code: string, error: string, meta?: Record<string, number>) {
	return { error, code, meta }
}

async function submitAccount() {
	await userEvent.click(submitButton())
	return screen.findByRole('alert')
}

test('sends the trimmed name and email and the password exactly as typed', async () => {
	let body: unknown
	server.use(
		http.post('/api/auth/setup', async ({ request }) => {
			body = await request.json()
			return HttpResponse.json(createdAccount, { status: 201 })
		}),
	)
	const { onCreated } = await renderReady()
	await fillAccount('  Maria Perez ', ' maria@example.com ')
	typePassword('  correct horse battery  ')

	await userEvent.click(submitButton())

	await waitFor(() => expect(onCreated).toHaveBeenCalled())
	expect(body).toEqual({
		code: setupCode,
		email: 'maria@example.com',
		name: 'Maria Perez',
		password: '  correct horse battery  ',
	})
})

test('locks the submit while the request is in flight', async () => {
	const posts = holdAccounts()
	await renderReady()
	await fillAccount()

	await userEvent.dblClick(submitButton())

	await waitFor(() => expect(posts()).toBe(1))
	expectSubmitOn(false)
	await userEvent.click(submitButton())
	expect(posts()).toBe(1)
})

test('shows its busy state while the request runs', async () => {
	const busy = busyClasses()
	expect(busy.length).toBeGreaterThan(0)
	holdAccounts()
	await renderReady()
	await fillAccount()

	await userEvent.click(submitButton())

	await waitFor(() => {
		for (const token of busy) {
			expect([...submitButton().classList]).toContain(token)
		}
	})
})

test('keeps the submit off while Name or Your Email holds only spaces', async () => {
	await renderReady()

	await fillAccount('   ', 'maria@example.com')
	expectSubmitOn(false)

	await userEvent.clear(screen.getByLabelText('Name'))
	await userEvent.clear(screen.getByLabelText('Your Email'))
	await fillAccount('Maria Perez', '   ')
	expectSubmitOn(false)
})

test('keeps a blocked password from going out with Enter', async () => {
	const posts = holdAccounts()
	const held = holdMeter()
	renderStep()
	await held.land(fakeMeter({ password1234: 1 }))
	await fillAccount()
	typePassword('password1234')

	await userEvent.type(screen.getByLabelText('Your Email'), '{Enter}')

	expect(posts()).toBe(0)
})

test("shows WordPress's sentence for an invalid email, above every row", async () => {
	answerAccount(422, refusal('email_invalid', 'invalid email address'))
	const { container } = await renderReady({ before: <p>Site Title row</p> })
	await fillAccount('Maria Perez', 'maria@example')

	const alert = await submitAccount()

	expect(alert).toHaveTextContent(
		'Sorry, that is not a valid email address. Email addresses look like username@example.com.',
	)
	expect(within(alert).getByText('username@example.com').tagName).toBe('CODE')
	expectInOrder(alert, screen.getByText('Site Title row'), screen.getByLabelText('Name'))
	expect(container).not.toHaveTextContent('invalid email address')
})

test('shows the hint with the floor the server asked for', async () => {
	await renderReady()
	await fillAccount()

	for (const min of [16, 12, 1]) {
		answerAccount(422, refusal('password_too_short', 'password must be at least 12 characters', { min }))
		const unit = min === 1 ? 'character' : 'characters'

		const alert = await submitAccount()

		await waitFor(() =>
			expect(alert).toHaveTextContent(`Hint: The password should be at least ${min} ${unit} long. ${hintTail}`),
		)
	}
})

test('says sorry for a short password refusal without its floor', async () => {
	answerAccount(422, { error: 'password too short', code: 'password_too_short' })
	await renderReady()
	await fillAccount()

	const alert = await submitAccount()

	expect(alert).toHaveTextContent(failedSentence)
})

test("tells the name and password refusals in WordPress's words", async () => {
	const refusals = [
		{ body: refusal('name_required', 'name is required'), sentence: 'Please provide a name.' },
		{
			body: refusal('name_too_long', 'name must be at most 256 characters', { max: 256 }),
			sentence: 'This name is too long.',
		},
		{
			body: refusal('password_too_long', 'password must be at most 1024 characters', { max: 1024 }),
			sentence: 'This password is too long.',
		},
	]
	const { container } = await renderReady()
	await fillAccount()

	for (const row of refusals) {
		answerAccount(422, row.body)

		await userEvent.click(submitButton())

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(row.sentence))
		expect(container).not.toHaveTextContent(row.body.error)
	}
})

test('asks to try again later when throttled, and says sorry when the site fails', async () => {
	await renderReady()
	await fillAccount()

	server.use(firstAccountRateLimited())
	expect(await submitAccount()).toHaveTextContent('Too many attempts. Please try again later.')

	answerAccount(500, refusal('internal', 'internal error'))
	await userEvent.click(submitButton())
	await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(failedSentence))
})

test("says sorry for a refusal it does not know, never in the server's words", async () => {
	const { container } = await renderReady()
	await fillAccount()

	for (const body of [{ error: 'role not configured' }, refusal('role_unknown', 'unknown role')]) {
		answerAccount(422, body)

		await userEvent.click(submitButton())

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(failedSentence))
		expect(container).not.toHaveTextContent(body.error)
	}
})

test('says the site is starting on a 503 and keeps the values', async () => {
	server.use(firstAccountStarting(), setupStatus(true))
	await renderReady()
	await fillAccount()
	typePassword('  correct horse battery  ')

	expect(await submitAccount()).toHaveTextContent(startingSentence)
	expect(screen.getByLabelText('Name')).toHaveValue('Maria Perez')
	expect(screen.getByLabelText('Your Email')).toHaveValue('maria@example.com')
	expect(screen.getByLabelText('Password')).toHaveValue('  correct horse battery  ')
})

test('asks the site once whether setup is still open after a 503', async () => {
	let probes = 0
	server.use(
		firstAccountStarting(),
		http.get('/api/auth/setup', () => {
			probes += 1
			return HttpResponse.json({ open: false })
		}),
	)
	const onClosed = vi.fn()
	await renderReady({ onClosed })
	await fillAccount()

	await submitAccount()

	await waitFor(() => expect(onClosed).toHaveBeenCalledOnce())
	expect(probes).toBe(1)
})

test('asks the site whether setup is still open after a network failure', async () => {
	server.use(
		http.post('/api/auth/setup', () => HttpResponse.error()),
		setupStatus(false),
	)
	const onClosed = vi.fn()
	await renderReady({ onClosed })
	await fillAccount()

	expect(await submitAccount()).toHaveTextContent(failedSentence)
	await waitFor(() => expect(onClosed).toHaveBeenCalledOnce())
})

test('keeps its sentence when the site still waits or cannot answer', async () => {
	const onClosed = vi.fn()
	await renderReady({ onClosed })
	await fillAccount()

	for (const probe of [setupStatus(true), http.get('/api/auth/setup', () => HttpResponse.json({}, { status: 500 }))]) {
		server.use(firstAccountStarting(), probe)

		await userEvent.click(submitButton())

		await waitFor(() => expectSubmitOn(true))
		expect(screen.getByRole('alert')).toHaveTextContent(startingSentence)
	}
	expect(onClosed).not.toHaveBeenCalled()
})

test('asks nothing more after a refusal that cannot have made the account', async () => {
	let probes = 0
	server.use(
		http.get('/api/auth/setup', () => {
			probes += 1
			return HttpResponse.json({ open: true })
		}),
	)
	const answers = [
		() => answerAccount(422, refusal('email_invalid', 'invalid email address')),
		() => server.use(firstAccountRateLimited()),
		() => server.use(firstAccountCodeRefused('expired')),
		() => answerAccount(500, refusal('internal', 'internal error')),
	]
	await renderReady()
	await fillAccount()

	for (const answer of answers) {
		answer()

		await userEvent.click(submitButton())

		await waitFor(() => expectSubmitOn(true))
	}
	expect(probes).toBe(0)
})

test('keeps every typed value after each refusal', async () => {
	const answers = [
		() => answerAccount(422, refusal('email_invalid', 'invalid email address')),
		() => server.use(firstAccountRateLimited()),
		() => server.use(firstAccountCodeRefused('expired')),
		() => answerAccount(500, refusal('internal', 'internal error')),
	]
	await renderReady()
	await fillAccount()
	typePassword('  correct horse battery  ')

	for (const answer of answers) {
		answer()

		await userEvent.click(submitButton())

		await waitFor(() => expectSubmitOn(true))
		expect(screen.getByLabelText('Name')).toHaveValue('Maria Perez')
		expect(screen.getByLabelText('Your Email')).toHaveValue('maria@example.com')
		expect(screen.getByLabelText('Password')).toHaveValue('  correct horse battery  ')
	}
})

test('shows the password in clear again after a refusal, as WordPress does after its reload', async () => {
	const answers = [
		() => answerAccount(422, refusal('password_too_short', 'password must be at least 12 characters', { min: 12 })),
		() => server.use(firstAccountRateLimited()),
		() => server.use(firstAccountCodeRefused('expired')),
		() => answerAccount(500, refusal('internal', 'internal error')),
	]
	await renderReady()
	await fillAccount()

	for (const answer of answers) {
		answer()
		await userEvent.click(screen.getByRole('button', { name: 'Hide password' }))

		await userEvent.click(submitButton())

		await waitFor(() => expect(screen.getByLabelText('Password')).toHaveAttribute('type', 'text'))
		expect(screen.getByRole('button', { name: 'Hide password' })).toHaveTextContent(/^Hide$/)
	}
})

test("draws a refusal in WordPress's message box", async () => {
	server.use(firstAccountRateLimited())
	await renderReady()
	await fillAccount()

	expect(await submitAccount()).toHaveClass('gopherium-setup__message')
})

const fourFields = { code: setupCode, email: 'maria@example.com', name: 'Maria Perez', password: 'twelvechars1' }

function countAccountPosts() {
	let posts = 0
	server.use(
		http.post('/api/auth/setup', () => {
			posts += 1
			return HttpResponse.json(createdAccount, { status: 201 })
		}),
	)
	return () => posts
}

test('hands the created account to the wizard', async () => {
	server.use(firstAccountOk())
	const { onCreated } = await renderReady()
	await fillAccount()

	await userEvent.click(submitButton())

	await waitFor(() => expect(onCreated).toHaveBeenCalledOnce())
	expect(onCreated).toHaveBeenCalledWith(createdAccount)
})

test('offers to try again when the wizard cannot move on, without posting again', async () => {
	const posts = countAccountPosts()
	const onCreated = vi.fn().mockRejectedValueOnce(new Error('the wizard could not move on'))
	await renderReady({ onCreated })
	await fillAccount()

	await userEvent.click(submitButton())

	expect(await screen.findByRole('alert')).toHaveTextContent('Something went wrong.')
	await userEvent.click(screen.getByRole('button', { name: 'Try again' }))
	await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(2))
	expect(onCreated).toHaveBeenLastCalledWith(createdAccount)
	expect(posts()).toBe(1)
})

test('stays busy while the wizard takes the account, and never sends it twice', async () => {
	const busy = busyClasses()
	const posts = countAccountPosts()
	let handOver: () => void = () => {}
	const onCreated = vi.fn(
		() =>
			new Promise<void>((resolve) => {
				handOver = resolve
			}),
	)
	await renderReady({ onCreated })
	await fillAccount()

	await userEvent.click(submitButton())

	await waitFor(() => expect(onCreated).toHaveBeenCalledOnce())
	for (const token of busy) {
		expect([...submitButton().classList]).toContain(token)
	}
	await act(async () => handOver())
	expectSubmitOn(false)
	await userEvent.click(submitButton())
	expect(posts()).toBe(1)
})

test('tells each refused code in its own words and hands the reason to the wizard', async () => {
	const reasons = [
		{
			reason: 'invalid',
			sentence: 'Your setup code appears to be invalid. Please copy it again from the setup code file.',
		},
		{ reason: 'expired', sentence: 'Your setup code has expired. A new one is waiting in the setup code file.' },
		{ reason: 'file', sentence: 'The setup code file cannot be used. Please check it and try again.' },
	] as const
	const onCodeRefused = vi.fn()
	await renderReady({ onCodeRefused })
	await fillAccount()

	for (const row of reasons) {
		server.use(firstAccountCodeRefused(row.reason))

		await userEvent.click(submitButton())

		await waitFor(() => expect(onCodeRefused).toHaveBeenLastCalledWith(row.reason))
		expect(screen.getByRole('alert')).toHaveTextContent(row.sentence)
	}
	expect(onCodeRefused).toHaveBeenCalledTimes(3)
})

test('reports a site that is already set up', async () => {
	server.use(firstAccountClosed())
	const onClosed = vi.fn()
	await renderReady({ onClosed })
	await fillAccount()

	expect(await submitAccount()).toHaveTextContent('You appear to have already set up this site.')
	await waitFor(() => expect(onClosed).toHaveBeenCalledOnce())
})

test("lets the wizard send the account with its own rows through its own route", async () => {
	const posts = countAccountPosts()
	const onSubmit = vi.fn(async () => ({ ...createdAccount, name: 'Maria Perez of the wizard' }))
	const { onCreated } = await renderReady({ onSubmit })
	await fillAccount()
	typePassword('twelvechars1')

	await userEvent.click(submitButton())

	await waitFor(() => expect(onCreated).toHaveBeenCalledOnce())
	expect(onSubmit.mock.calls).toStrictEqual([[fourFields]])
	expect(onCreated).toHaveBeenCalledWith({ ...createdAccount, name: 'Maria Perez of the wizard' })
	expect(posts()).toBe(0)
})

test("shows the wizard's words for an error the step does not know", async () => {
	const failure = new Error('the database test failed')
	const onSubmit = vi.fn(async () => {
		throw failure
	})
	const onError = vi.fn(() => 'The database could not be reached.')
	await renderReady({ onSubmit, onError })
	await fillAccount()

	expect(await submitAccount()).toHaveTextContent('The database could not be reached.')
	expect(onError).toHaveBeenCalledWith(failure)
})

test("says sorry for the wizard's error when the wizard has no words for it", async () => {
	const onSubmit = vi.fn(async () => {
		throw new Error('the database test failed')
	})
	await renderReady({ onSubmit, onError: () => undefined })
	await fillAccount()

	expect(await submitAccount()).toHaveTextContent(failedSentence)
})

test('meets every refusal without the optional callbacks', async () => {
	const stray = vi.fn()
	process.on('unhandledRejection', stray)
	const answers = [
		() => server.use(firstAccountCodeRefused('expired')),
		() => server.use(firstAccountClosed()),
		() => server.use(firstAccountStarting(), setupStatus(false)),
		() => server.use(firstAccountRateLimited()),
		() => server.use(http.post('/api/auth/setup', () => HttpResponse.error()), setupStatus(false)),
	]
	await renderReady()
	await fillAccount()

	for (const answer of answers) {
		answer()

		await userEvent.click(submitButton())

		await waitFor(() => expectSubmitOn(true))
		expect(screen.getByRole('alert')).toBeInTheDocument()
	}
	await new Promise((resolve) => setTimeout(resolve))
	process.off('unhandledRejection', stray)
	expect(stray).not.toHaveBeenCalled()
})

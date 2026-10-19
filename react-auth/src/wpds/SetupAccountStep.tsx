// SPDX-License-Identifier: Apache-2.0

import { useMutation } from '@tanstack/react-query'
import { __, _n, _x, sprintf } from '@wordpress/i18n'
import type { TransformedText } from '@wordpress/i18n'
import { Button, CheckboxControl, Field, Input, Text } from '@wordpress/ui'
import { useEffect, useEffectEvent, useId, useRef, useState } from 'react'
import type { ComponentProps, KeyboardEvent, ReactNode } from 'react'

import type { Account } from '../account.js'
import { ValidationError } from '../api.js'
import { DOMAIN } from '../domain.js'
import { SetupClosedError, SetupCodeError, SiteStartingError, createFirstAccount, isSetupOpen } from '../setup.js'
import type { FirstAccount } from '../setup.js'
import { generatePassword } from './password.js'
import { setupRefusal } from './setupWords.js'
import { disallowedWords, levelOf, loadMeter } from './strength.js'
import type { Meter, Tone } from './strength.js'

/** The length of the password the step starts with, unless the wizard asks for another. */
const GENERATED_PASSWORD_LENGTH = 18

/** The word the meter box holds for an empty password. */
const NO_WORD = '\u00a0'

/** What the meter knows of the password: nothing yet, an empty field, or a score from -1 to 4. */
type Strength = number | 'empty' | undefined

/** The password, what the meter knows of it and whether its weak use is confirmed. */
interface HeldPassword {
	password: string
	strength: Strength
	confirmed: boolean
}

/** The wizard's words for an error the step does not know. */
type ErrorWords = (error: unknown) => ReactNode | undefined

/** The wizard's optional callbacks for what a refusal means for it. */
interface RefusalHooks {
	onCodeRefused?: (reason: SetupCodeError['reason']) => void
	onClosed?: () => void
}

/**
 * Renders a sentence with the part between its code tags as code.
 * @param sentence - The translated sentence.
 * @returns The sentence with its code part as markup.
 */
function withCode(sentence: string): ReactNode {
	return sentence.split(/<\/?code>/).map((part, index) => (index % 2 === 1 ? <code key={index}>{part}</code> : part))
}

/**
 * Returns the password hint with the server's floor as its number.
 * @param min - The fewest characters the server accepts.
 * @returns The hint.
 */
function passwordHint(min: number): string {
	const hint = _n(
		'Hint: The password should be at least %d character long. ' +
			'To make it stronger, use upper and lower case letters, numbers, and symbols like ! " ? $ %% ^ & ).',
		'Hint: The password should be at least %d characters long. ' +
			'To make it stronger, use upper and lower case letters, numbers, and symbols like ! " ? $ %% ^ & ).',
		min,
		DOMAIN,
	) as TransformedText<'%d'>
	return sprintf(hint, min)
}

const validationWords = new Map<string, (meta: Record<string, unknown>) => ReactNode>([
	[
		'email_invalid',
		() =>
			withCode(
				__(
					'Sorry, that is not a valid email address. ' +
						'Email addresses look like <code>username@example.com</code>.',
					DOMAIN,
				),
			),
	],
	['name_required', () => __('Please provide a name.', DOMAIN)],
	['name_too_long', () => __('This name is too long.', DOMAIN)],
	['password_too_short', (meta) => (typeof meta.min === 'number' ? passwordHint(meta.min) : undefined)],
	['password_too_long', () => __('This password is too long.', DOMAIN)],
])

/**
 * Returns the sentence for a validation refusal the step knows.
 * @param error - The error the submit raised.
 * @returns The sentence, or undefined for any other error.
 */
function validationRefusal(error: Error): ReactNode | undefined {
	return error instanceof ValidationError ? validationWords.get(error.code ?? '')?.(error.meta ?? {}) : undefined
}

/**
 * Returns the sentence the step shows for a refused submit.
 * @param error - The error the submit raised.
 * @param onError - The wizard's words for an error the step does not know, if it has any.
 * @returns The sentence to show.
 */
function refusalMessage(error: Error, onError?: ErrorWords): ReactNode {
	return (
		setupRefusal(error) ??
		validationRefusal(error) ??
		onError?.(error) ??
		__('The account could not be created, please try again.', DOMAIN)
	)
}

/**
 * Reports whether a failed submit may already have made the account.
 * @param error - The error the submit raised.
 * @returns True after a 503 or a network failure.
 */
function mayHaveCreated(error: Error): boolean {
	return error instanceof SiteStartingError || error instanceof TypeError
}

/**
 * Asks the site once whether it still waits for its first account, and tells the wizard when it does not.
 * @param onClosed - Called when the site answers that setup is closed.
 */
async function probeClosed(onClosed?: () => void): Promise<void> {
	const open = await isSetupOpen().catch(() => true)
	if (!open) {
		onClosed?.()
	}
}

/**
 * Tells the wizard what a refused submit means for it.
 * @param error - The error the submit raised.
 * @param hooks - The wizard's optional callbacks.
 */
async function reportRefusal(error: Error, hooks: RefusalHooks): Promise<void> {
	if (error instanceof SetupCodeError) {
		hooks.onCodeRefused?.(error.reason)
	} else if (error instanceof SetupClosedError) {
		hooks.onClosed?.()
	} else if (mayHaveCreated(error)) {
		await probeClosed(hooks.onClosed)
	}
}

/**
 * Returns what the meter knows of a password at the moment it is scored.
 * @param password - The password as typed.
 * @param meter - The meter, once it has landed.
 * @param words - Reads the words the meter treats as easy to guess.
 * @returns The strength, -1 while the meter has not landed.
 */
function strengthOf(password: string, meter: Meter | undefined, words: () => string[]): Strength {
	if (password.trim() === '') {
		return 'empty'
	}
	return meter === undefined ? -1 : meter(password, words())
}

/**
 * Returns the meter's tone for a strength.
 * @param strength - What the meter knows of the password.
 * @returns The tone, or undefined while nothing was scored or the field is empty.
 */
function toneOf(strength: Strength): Tone | undefined {
	return typeof strength === 'number' ? levelOf(strength).tone : undefined
}

/**
 * Reports whether a password this strong needs its use confirmed.
 * @param strength - What the meter knows of the password.
 * @returns True for the very weak, weak and unknown levels.
 */
function needsConfirmation(strength: Strength): boolean {
	const tone = toneOf(strength)
	return tone === 'short' || tone === 'bad'
}

/**
 * Returns the word the meter box holds.
 * @param strength - What the meter knows of the password.
 * @returns Nothing before scoring, a no-break space for an empty field, or the level's word.
 */
function meterWord(strength: Strength): string {
	if (strength === undefined) {
		return ''
	}
	return strength === 'empty' ? NO_WORD : levelOf(strength).word
}

/**
 * Returns a class with its tone modifier when it has a tone.
 * @param block - The class without a tone.
 * @param tone - The tone to add, if any.
 * @returns The class names.
 */
function withTone(block: string, tone: Tone | undefined): string {
	return tone === undefined ? block : `${block} ${block}--${tone}`
}

/**
 * Reports whether a key types a character, the only keys Caps Lock is read from.
 * @param event - The key the person pressed.
 * @returns True for a printable key pressed without Control, Alt or Meta.
 */
function isPrintable(event: KeyboardEvent): boolean {
	return event.key.length === 1 && !event.ctrlKey && !event.altKey && !event.metaKey
}

/**
 * Returns the words the meter treats as easy to guess, read from the fields when it scores.
 * @param name - The name as typed.
 * @param email - The email as typed.
 * @param userInputs - The wizard's own values, if it gave any.
 * @returns The distinct words of four characters or more.
 */
function wordsFor(name: string, email: string, userInputs: string[] = []): string[] {
	return disallowedWords([document.title, location.href, name, email, ...userInputs])
}

/**
 * Reports whether the step may send the account, the meter's confirmation included.
 * @param name - The name as typed.
 * @param email - The email as typed.
 * @param held - The password and what the meter knows of it.
 * @returns True once every field holds more than spaces and a weak password is confirmed.
 */
function isReady(name: string, email: string, held: HeldPassword): boolean {
	return (
		name.trim() !== '' &&
		email.trim() !== '' &&
		held.password.trim() !== '' &&
		(held.confirmed || !needsConfirmation(held.strength))
	)
}

/**
 * Holds the password, scores it when the meter lands and on each change, and keeps the weak use confirmation.
 * @param length - The length of the password the step starts with, if the wizard chose one.
 * @param words - Reads the words the meter treats as easy to guess.
 * @returns The password, its strength, the confirmation and their setters.
 */
function usePassword(length: number | undefined, words: () => string[]) {
	const [password, setPassword] = useState(() => generatePassword(length ?? GENERATED_PASSWORD_LENGTH))
	const [strength, setStrength] = useState<Strength>(undefined)
	const [confirmed, setConfirmed] = useState(false)
	const meter = useRef<Meter>(undefined)
	const landed = useEffectEvent((loaded: Meter) => {
		meter.current = loaded
		setStrength(strengthOf(password, loaded, words))
	})
	useEffect(() => {
		loadMeter().then(
			(loaded) => landed(loaded),
			() => undefined,
		)
	}, [])
	const change = (value: string) => {
		setPassword(value)
		setStrength(strengthOf(value, meter.current, words))
		if (value.trim() === '') {
			setConfirmed(false)
		}
	}
	return { password, strength, confirmed, change, setConfirmed }
}

/**
 * Sends the first account once and hands it to the wizard, retrying only the hand-over.
 * @param send - Sends the account, through the wizard's route or the transport.
 * @param onCreated - Called with the account once the site created it.
 * @param onRefused - Called first after every refused submit.
 * @param hooks - The wizard's optional callbacks for a refusal.
 * @returns The submit, its refusal, its busy and locked states, and the retry once a hand-over failed.
 */
function useFirstAccount(
	send: () => Promise<Account>,
	onCreated: (account: Account) => void | Promise<void>,
	onRefused: () => void,
	hooks: RefusalHooks,
) {
	const [created, setCreated] = useState<Account | null>(null)
	const [handoffFailed, setHandoffFailed] = useState(false)
	const finish = useMutation({
		mutationFn: async (account: Account) => {
			await onCreated(account)
		},
		onError: () => setHandoffFailed(true),
		onSuccess: () => setHandoffFailed(false),
	})
	const create = useMutation({
		mutationFn: send,
		onSuccess: (account) => {
			setCreated(account)
			finish.mutate(account)
		},
		onError: async (error) => {
			onRefused()
			await reportRefusal(error, hooks)
		},
	})
	return {
		submit: () => create.mutate(),
		error: create.error,
		busy: create.isPending || finish.isPending,
		locked: create.isPending || create.isSuccess,
		retry: created !== null && handoffFailed ? () => finish.mutate(created) : undefined,
		retrying: finish.isPending,
	}
}

/**
 * Renders a sentence in the message box above the rows.
 * @param children - The sentence.
 * @returns The message box element.
 */
function MessageBox({ children }: { children: ReactNode }) {
	return (
		<Text role="alert" render={<p className="gopherium-setup__message" />}>
			{children}
		</Text>
	)
}

/**
 * Renders the sentence for a refused submit, if there is one.
 * @param error - The error the last submit raised, if any.
 * @param onError - The wizard's words for an error the step does not know.
 * @returns The message box element, or nothing.
 */
function RefusalMessage({ error, onError }: { error: Error | null; onError?: ErrorWords }) {
	return error === null ? null : <MessageBox>{refusalMessage(error, onError)}</MessageBox>
}

/**
 * Renders one labelled field row of the step, with its help line when it has one.
 * @param label - The field's label.
 * @param help - The line under the field, if any.
 * @returns The row element.
 */
function TextRow({ label, help, ...field }: { label: string; help?: string } & ComponentProps<typeof Input>) {
	return (
		<Field.Root className="gopherium-setup__row">
			<Field.Label>{label}</Field.Label>
			<div>
				<Input {...field} />
				{help === undefined ? null : <Field.Description className="gopherium-setup__help">{help}</Field.Description>}
			</div>
		</Field.Root>
	)
}

/**
 * Renders the button that shows or hides the password.
 * @param masked - Whether the password is hidden.
 * @param onToggle - Called when the button is pressed.
 * @returns The toggle element.
 */
function MaskToggle({ masked, onToggle }: { masked: boolean; onToggle: () => void }) {
	return (
		<Button
			type="button"
			variant="outline"
			aria-label={masked ? __('Show password', DOMAIN) : __('Hide password', DOMAIN)}
			onClick={onToggle}
		>
			{masked ? __('Show', DOMAIN) : __('Hide', DOMAIN)}
		</Button>
	)
}

/**
 * Renders the box that shows the meter's word in its tone.
 * @param id - The id the password field is described by.
 * @param strength - What the meter knows of the password.
 * @returns The meter box element.
 */
function MeterBox({ id, strength }: { id: string; strength: Strength }) {
	return (
		<div id={id} aria-live="polite" className={withTone('gopherium-setup__meter', toneOf(strength))}>
			{meterWord(strength)}
		</div>
	)
}

/**
 * Renders the password row with the meter box, the Caps Lock line and the line asking to store it.
 * @param password - The password as typed.
 * @param strength - What the meter knows of the password.
 * @param masked - Whether the password is hidden.
 * @param onPasswordChange - Called with the password on every change.
 * @param onToggle - Called when the show or hide button is pressed.
 * @returns The row element.
 */
function PasswordRow({
	password,
	strength,
	masked,
	onPasswordChange,
	onToggle,
}: {
	password: string
	strength: Strength
	masked: boolean
	onPasswordChange: (password: string) => void
	onToggle: () => void
}) {
	const [capsLock, setCapsLock] = useState(false)
	const meterId = useId()
	const helpId = useId()
	return (
		<Field.Root className="gopherium-setup__row">
			<Field.Label>{__('Password', DOMAIN)}</Field.Label>
			<div>
				<div className="gopherium-setup__password">
					<div className="gopherium-setup__password-field">
						<Input
							type={masked ? 'password' : 'text'}
							className={withTone('gopherium-setup__input', toneOf(strength))}
							value={password}
							onChange={(event) => onPasswordChange(event.target.value)}
							onKeyDown={(event) => {
								if (isPrintable(event)) {
									setCapsLock(event.getModifierState('CapsLock'))
								}
							}}
							autoComplete="new-password"
							spellCheck={false}
							aria-describedby={`${meterId} ${helpId}`}
						/>
						<MeterBox id={meterId} strength={strength} />
						<div aria-live="assertive" className="gopherium-setup__caps">
							{capsLock ? __('Caps lock is on.', DOMAIN) : null}
						</div>
					</div>
					<MaskToggle masked={masked} onToggle={onToggle} />
				</div>
				<Text render={<p id={helpId} className="gopherium-setup__help" />}>
					<strong>{__('Important:', DOMAIN)}</strong>{' '}
					{__('You will need this password to log\u00a0in. Please store it in a secure location.', DOMAIN)}
				</Text>
			</div>
		</Field.Root>
	)
}

/**
 * Renders the row asking to confirm the use of a weak password.
 * @param checked - Whether the use is confirmed.
 * @param onCheckedChange - Called with the new choice.
 * @returns The row element.
 */
function ConfirmRow({ checked, onCheckedChange }: { checked: boolean; onCheckedChange: (checked: boolean) => void }) {
	return (
		<div className="gopherium-setup__row">
			<Field.VisualLabel>{__('Confirm Password', DOMAIN)}</Field.VisualLabel>
			<div>
				<CheckboxControl
					label={__('Confirm use of weak password', DOMAIN)}
					checked={checked}
					onCheckedChange={(next) => onCheckedChange(next)}
				/>
			</div>
		</div>
	)
}

/**
 * Renders the sentence and the button that hand the created account to the wizard again.
 * @param pending - Whether the hand-over runs.
 * @param onRetry - Called when the button is pressed.
 * @returns The hand-over failure element.
 */
function HandoffFailure({ pending, onRetry }: { pending: boolean; onRetry: () => void }) {
	return (
		<div className="gopherium-setup">
			<MessageBox>{__('Something went wrong.', DOMAIN)}</MessageBox>
			<p className="gopherium-setup__step">
				<Button type="button" disabled={pending} loading={pending} onClick={onRetry}>
					{__('Try again', DOMAIN)}
				</Button>
			</p>
		</div>
	)
}

/**
 * Renders the first account's Name, Password and Your Email rows inside an application's setup wizard.
 * @param code - The one-time setup code the wizard collected before this step.
 * @param submitLabel - The wizard's words for the submit button.
 * @param onCreated - Called with the account once the site created it.
 * @param onSubmit - Sends the account through the wizard's own route, in place of the transport.
 * @param onError - The wizard's words for an error the step does not know.
 * @param onCodeRefused - Called with the reason when the site refuses the setup code.
 * @param onClosed - Called when the site already has its first account.
 * @param generatedPasswordLength - The length of the password the step starts with.
 * @param userInputs - The wizard's own values the meter treats as easy to guess.
 * @param before - The wizard's rows above Name.
 * @param after - The wizard's rows below Your Email.
 * @returns The setup account step element.
 */
export function SetupAccountStep({
	code,
	submitLabel,
	onCreated,
	onSubmit,
	onError,
	onCodeRefused,
	onClosed,
	generatedPasswordLength,
	userInputs,
	before,
	after,
}: {
	code: string
	submitLabel: string
	onCreated: (account: Account) => void | Promise<void>
	onSubmit?: (input: FirstAccount) => Promise<Account>
	onError?: ErrorWords
	onCodeRefused?: (reason: SetupCodeError['reason']) => void
	onClosed?: () => void
	generatedPasswordLength?: number
	userInputs?: string[]
	before?: ReactNode
	after?: ReactNode
}) {
	const [name, setName] = useState('')
	const [email, setEmail] = useState('')
	const [masked, setMasked] = useState(false)
	const held = usePassword(generatedPasswordLength, () => wordsFor(name, email, userInputs))
	const account = useFirstAccount(
		() => (onSubmit ?? createFirstAccount)({ code, email: email.trim(), name: name.trim(), password: held.password }),
		onCreated,
		() => setMasked(false),
		{ onCodeRefused, onClosed },
	)
	if (account.retry !== undefined) {
		return <HandoffFailure pending={account.retrying} onRetry={account.retry} />
	}
	return (
		<form
			className="gopherium-setup"
			noValidate
			onSubmit={(event) => {
				event.preventDefault()
				setMasked(true)
				account.submit()
			}}
		>
			<RefusalMessage error={account.error} onError={onError} />
			{before}
			<TextRow
				label={_x('Name', 'field label', DOMAIN)}
				type="text"
				autoComplete="name"
				value={name}
				onChange={(event) => setName(event.target.value)}
			/>
			<PasswordRow
				password={held.password}
				strength={held.strength}
				masked={masked}
				onPasswordChange={held.change}
				onToggle={() => setMasked(!masked)}
			/>
			{needsConfirmation(held.strength) ? (
				<ConfirmRow checked={held.confirmed} onCheckedChange={held.setConfirmed} />
			) : null}
			<TextRow
				label={__('Your Email', DOMAIN)}
				type="email"
				autoComplete="username"
				value={email}
				onChange={(event) => setEmail(event.target.value)}
				help={__('Double-check your email address before continuing.', DOMAIN)}
			/>
			{after}
			<p className="gopherium-setup__step">
				<Button type="submit" disabled={account.locked || !isReady(name, email, held)} loading={account.busy}>
					{submitLabel}
				</Button>
			</p>
		</form>
	)
}

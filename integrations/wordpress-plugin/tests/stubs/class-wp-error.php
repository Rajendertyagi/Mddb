<?php
/**
 * Minimal WP_Error stand-in for unit tests.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

/**
 * Mimics the API surface our production code touches (`get_error_code`,
 * `get_error_message`). Not a faithful reproduction of WordPress's WP_Error.
 */
class WP_Error { // phpcs:ignore
	private string $code;
	private string $message;
	/** @var array<string,mixed> */
	private array $data;

	/**
	 * @param array<string,mixed> $data
	 */
	public function __construct( string $code = '', string $message = '', array $data = [] ) {
		$this->code    = $code;
		$this->message = $message;
		$this->data    = $data;
	}

	public function get_error_code(): string { // phpcs:ignore
		return $this->code;
	}

	public function get_error_message(): string { // phpcs:ignore
		return $this->message;
	}

	/**
	 * @return array<string,mixed>
	 */
	public function get_error_data() { // phpcs:ignore
		return $this->data;
	}

	/**
	 * @param mixed $data
	 */
	public function add_data( $data ): void { // phpcs:ignore
		if ( is_array( $data ) ) {
			$this->data = array_replace( $this->data, $data );
		}
	}
}

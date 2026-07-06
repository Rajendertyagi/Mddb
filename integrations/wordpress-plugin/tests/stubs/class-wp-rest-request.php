<?php
/**
 * Minimal WP_REST_Request stand-in for unit tests.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

// phpcs:disable WordPress.NamingConventions

if ( class_exists( 'WP_REST_Request' ) ) {
	return;
}

/**
 * Carries headers + a JSON body, mirroring the two accessors the plugin uses.
 */
class WP_REST_Request {

	/** @var array<string,string> */
	private array $headers;

	/** @var mixed */
	private $json;

	/**
	 * @param array<string,string> $headers
	 * @param mixed                $json
	 */
	public function __construct( array $headers = [], $json = null ) {
		$normalised = [];
		foreach ( $headers as $name => $value ) {
			$normalised[ strtolower( str_replace( '-', '_', (string) $name ) ) ] = (string) $value;
		}
		$this->headers = $normalised;
		$this->json    = $json;
	}

	/** @return string|null */
	public function get_header( string $name ) {
		$key = strtolower( str_replace( '-', '_', $name ) );
		return $this->headers[ $key ] ?? null;
	}

	/** @return mixed */
	public function get_json_params() {
		return $this->json;
	}
}

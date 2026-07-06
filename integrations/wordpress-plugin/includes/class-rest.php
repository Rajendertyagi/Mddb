<?php
/**
 * REST routes MDDB's MCP tools call to publish into WordPress.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Inbound half of the MCP → WordPress publishing bridge:
 *
 *   POST /wp-json/mddb-sync/v1/publish  — create/update a post or page
 *   POST /wp-json/mddb-sync/v1/status   — change publishing status
 *
 * Both routes are OFF by default. They activate only when the "Remote
 * publishing" toggle is on AND a publish key is set; every request must
 * present that key as `Authorization: Bearer <key>` (or the
 * `X-MDDB-Publish-Key` header for proxies that strip Authorization).
 * Keys are compared with `hash_equals` to avoid timing side channels.
 */
final class Rest {

	public const ROUTE_NAMESPACE = 'mddb-sync/v1';

	private Settings $settings;

	private Publisher $publisher;

	public function __construct( Settings $settings, Publisher $publisher ) {
		$this->settings  = $settings;
		$this->publisher = $publisher;
	}

	public function register(): void {
		add_action( 'rest_api_init', [ $this, 'registerRoutes' ] );
	}

	public function registerRoutes(): void {
		register_rest_route(
			self::ROUTE_NAMESPACE,
			'/publish',
			[
				'methods'             => 'POST',
				'callback'            => [ $this, 'handlePublish' ],
				'permission_callback' => [ $this, 'checkAuth' ],
			]
		);
		register_rest_route(
			self::ROUTE_NAMESPACE,
			'/status',
			[
				'methods'             => 'POST',
				'callback'            => [ $this, 'handleStatus' ],
				'permission_callback' => [ $this, 'checkAuth' ],
			]
		);
	}

	/**
	 * @param \WP_REST_Request $request
	 * @return true|\WP_Error
	 */
	public function checkAuth( $request ) {
		if ( ! $this->settings->publishEnabled() ) {
			return new \WP_Error(
				'mddb_publish_disabled',
				'Remote publishing is disabled — enable it under Settings → MDDB Sync.',
				[ 'status' => 403 ]
			);
		}
		$key = $this->settings->publishKey();
		if ( $key === '' ) {
			return new \WP_Error(
				'mddb_publish_no_key',
				'No publish key configured — set one under Settings → MDDB Sync.',
				[ 'status' => 403 ]
			);
		}
		$provided = $this->keyFrom( $request );
		if ( $provided === '' || ! hash_equals( $key, $provided ) ) {
			return new \WP_Error( 'mddb_publish_unauthorized', 'Invalid or missing publish key.', [ 'status' => 401 ] );
		}
		return true;
	}

	/**
	 * @param \WP_REST_Request $request
	 * @return \WP_REST_Response|\WP_Error
	 */
	public function handlePublish( $request ) {
		$payload = $this->payloadFrom( $request );
		if ( is_wp_error( $payload ) ) {
			return $payload;
		}
		$result = $this->publisher->publish( $payload );
		if ( is_wp_error( $result ) ) {
			return $result;
		}
		return rest_ensure_response( $result );
	}

	/**
	 * @param \WP_REST_Request $request
	 * @return \WP_REST_Response|\WP_Error
	 */
	public function handleStatus( $request ) {
		$payload = $this->payloadFrom( $request );
		if ( is_wp_error( $payload ) ) {
			return $payload;
		}
		$result = $this->publisher->changeStatus( $payload );
		if ( is_wp_error( $result ) ) {
			return $result;
		}
		return rest_ensure_response( $result );
	}

	/**
	 * @param \WP_REST_Request $request
	 * @return array<string,mixed>|\WP_Error
	 */
	private function payloadFrom( $request ) {
		$payload = $request->get_json_params();
		if ( ! is_array( $payload ) || count( $payload ) === 0 ) {
			return new \WP_Error( 'mddb_publish_bad_payload', 'Request body must be a JSON object.', [ 'status' => 400 ] );
		}
		return $payload;
	}

	/**
	 * @param \WP_REST_Request $request
	 */
	private function keyFrom( $request ): string {
		$header = (string) $request->get_header( 'authorization' );
		if ( stripos( $header, 'bearer ' ) === 0 ) {
			return trim( substr( $header, 7 ) );
		}
		return trim( (string) $request->get_header( 'x-mddb-publish-key' ) );
	}
}

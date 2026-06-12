<?php
/**
 * Minimal WordPress function shims for unit tests.
 *
 * Brain Monkey replaces these with mocks per-test (`Brain\Monkey\Functions\when`).
 * The fallback bodies here matter only when a test path forgets to stub a call,
 * in which case we return sensible defaults instead of fatal errors.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

// phpcs:disable WordPress.NamingConventions

if ( ! function_exists( 'is_wp_error' ) ) {
	function is_wp_error( $thing ): bool {
		return $thing instanceof WP_Error;
	}
}
if ( ! function_exists( 'sanitize_key' ) ) {
	function sanitize_key( $key ): string {
		$key = is_string( $key ) ? $key : '';
		$key = strtolower( $key );
		return (string) preg_replace( '/[^a-z0-9_\-]/', '', $key );
	}
}
if ( ! function_exists( 'sanitize_title' ) ) {
	function sanitize_title( $title ): string {
		$title = is_string( $title ) ? $title : '';
		$title = strtolower( $title );
		$title = (string) preg_replace( '/[^a-z0-9]+/', '-', $title );
		return trim( $title, '-' );
	}
}
if ( ! function_exists( 'esc_url_raw' ) ) {
	function esc_url_raw( $url ): string {
		return is_string( $url ) ? $url : '';
	}
}
if ( ! function_exists( 'untrailingslashit' ) ) {
	function untrailingslashit( $value ): string {
		return is_string( $value ) ? rtrim( $value, '/\\' ) : '';
	}
}
if ( ! function_exists( 'wp_http_validate_url' ) ) {
	function wp_http_validate_url( $url ): bool {
		return is_string( $url ) && (bool) filter_var( $url, FILTER_VALIDATE_URL );
	}
}
if ( ! function_exists( 'wp_parse_url' ) ) {
	function wp_parse_url( $url, $component = -1 ) {
		return is_string( $url ) ? parse_url( $url, $component ) : false;
	}
}
if ( ! function_exists( 'wp_json_encode' ) ) {
	function wp_json_encode( $data ): string {
		return (string) json_encode( $data );
	}
}
if ( ! function_exists( 'wp_strip_all_tags' ) ) {
	function wp_strip_all_tags( $string, $remove_breaks = false ): string {
		$string = is_string( $string ) ? strip_tags( $string ) : '';
		return $remove_breaks ? trim( (string) preg_replace( '/\s+/', ' ', $string ) ) : $string;
	}
}
if ( ! function_exists( 'wp_is_post_autosave' ) ) {
	function wp_is_post_autosave( $id ): bool { unset( $id ); return false; }
}
if ( ! function_exists( 'wp_is_post_revision' ) ) {
	function wp_is_post_revision( $id ): bool { unset( $id ); return false; }
}
if ( ! function_exists( 'get_locale' ) ) {
	function get_locale(): string { return 'en_US'; }
}
if ( ! function_exists( 'home_url' ) ) {
	function home_url(): string { return 'https://example.com'; }
}
if ( ! function_exists( 'get_post_type' ) ) {
	function get_post_type( $post = null ): string {
		if ( $post instanceof WP_Post ) {
			return $post->post_type;
		}
		return 'post';
	}
}
if ( ! function_exists( 'get_post' ) ) {
	function get_post( $id ) { unset( $id ); return null; }
}
if ( ! function_exists( 'get_permalink' ) ) {
	function get_permalink( $post ): string {
		if ( $post instanceof WP_Post ) {
			return 'https://example.com/?p=' . $post->ID;
		}
		return '';
	}
}
if ( ! function_exists( 'get_post_time' ) ) {
	function get_post_time( $format, $gmt, $post ): string { unset( $format, $gmt, $post ); return '2026-05-19T00:00:00+00:00'; }
}
if ( ! function_exists( 'get_post_modified_time' ) ) {
	function get_post_modified_time( $format, $gmt, $post ): string { unset( $format, $gmt, $post ); return '2026-05-19T00:00:00+00:00'; }
}
if ( ! function_exists( 'get_the_author_meta' ) ) {
	function get_the_author_meta( $field, $user_id ): string { unset( $field, $user_id ); return ''; }
}
if ( ! function_exists( 'get_the_terms' ) ) {
	function get_the_terms( $post, $taxonomy ) { unset( $post, $taxonomy ); return []; }
}
if ( ! function_exists( '__' ) ) {
	function __( $text, $domain = null ): string { unset( $domain ); return (string) $text; }
}
if ( ! function_exists( '_e' ) ) {
	function _e( $text, $domain = null ): void { unset( $domain ); echo (string) $text; }
}
if ( ! function_exists( 'esc_html__' ) ) {
	function esc_html__( $text, $domain = null ): string { unset( $domain ); return htmlspecialchars( (string) $text ); }
}
if ( ! function_exists( 'esc_attr__' ) ) {
	function esc_attr__( $text, $domain = null ): string { unset( $domain ); return htmlspecialchars( (string) $text ); }
}
if ( ! function_exists( 'esc_html' ) ) {
	function esc_html( $text ): string { return htmlspecialchars( (string) $text ); }
}
if ( ! function_exists( 'esc_attr' ) ) {
	function esc_attr( $text ): string { return htmlspecialchars( (string) $text ); }
}
if ( ! function_exists( 'esc_url' ) ) {
	function esc_url( $url ): string { return is_string( $url ) ? $url : ''; }
}
if ( ! function_exists( 'add_settings_error' ) ) {
	function add_settings_error( $setting, $code, $message, $type = 'error' ): void { unset( $setting, $code, $message, $type ); }
}
if ( ! function_exists( 'wp_remote_post' ) ) {
	function wp_remote_post( $url, $args = [] ) { unset( $url, $args ); return [ 'response' => [ 'code' => 200, 'message' => 'OK' ], 'body' => '{}' ]; }
}
if ( ! function_exists( 'wp_remote_get' ) ) {
	function wp_remote_get( $url, $args = [] ) { unset( $url, $args ); return [ 'response' => [ 'code' => 200, 'message' => 'OK' ], 'body' => '{}' ]; }
}
if ( ! function_exists( 'wp_remote_retrieve_response_code' ) ) {
	function wp_remote_retrieve_response_code( $response ): int {
		return is_array( $response ) && isset( $response['response']['code'] ) ? (int) $response['response']['code'] : 0;
	}
}
if ( ! function_exists( 'wp_remote_retrieve_body' ) ) {
	function wp_remote_retrieve_body( $response ): string {
		return is_array( $response ) && isset( $response['body'] ) ? (string) $response['body'] : '';
	}
}
if ( ! function_exists( 'get_option' ) ) {
	function get_option( $name, $default = false ) { unset( $name ); return $default; }
}
if ( ! function_exists( 'add_option' ) ) {
	function add_option( $name, $value ) { unset( $name, $value ); return true; }
}
if ( ! function_exists( 'delete_site_transient' ) ) {
	function delete_site_transient( $name ): bool { unset( $name ); return true; }
}
if ( ! function_exists( 'set_site_transient' ) ) {
	function set_site_transient( $name, $value, $ttl ): bool { unset( $name, $value, $ttl ); return true; }
}
if ( ! function_exists( 'get_site_transient' ) ) {
	function get_site_transient( $name ) { unset( $name ); return false; }
}
if ( ! function_exists( 'apply_filters' ) ) {
	function apply_filters( $hook, $value ) { unset( $hook ); return $value; }
}
if ( ! function_exists( 'do_action' ) ) {
	function do_action( ...$args ): void { unset( $args ); }
}
if ( ! function_exists( 'add_action' ) ) {
	function add_action( ...$args ): bool { unset( $args ); return true; }
}
if ( ! function_exists( 'add_filter' ) ) {
	function add_filter( ...$args ): bool { unset( $args ); return true; }
}
if ( ! function_exists( 'load_plugin_textdomain' ) ) {
	function load_plugin_textdomain( ...$args ): bool { unset( $args ); return true; }
}
if ( ! function_exists( 'register_setting' ) ) {
	function register_setting( ...$args ): void { unset( $args ); }
}
if ( ! function_exists( 'plugin_dir_path' ) ) {
	function plugin_dir_path( $file ): string { return rtrim( dirname( (string) $file ), '/' ) . '/'; }
}
if ( ! function_exists( 'plugin_basename' ) ) {
	function plugin_basename( $file ): string { return basename( (string) $file ); }
}

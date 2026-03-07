import axios, { AxiosError, AxiosResponse } from 'axios';
import type { ApiError } from '@/types';
import { useAuthStore } from '@/store/auth-store';
import { CURRENT_API_VERSION, createVersionHeaders, parseVersionResponse, VersionResponse } from './versions';
import { validateApiResponse } from './validation';

const apiClient = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  headers: {
    'Content-Type': 'application/json',
    ...createVersionHeaders(CURRENT_API_VERSION),
  },
});

// Attach JWT token to every request
apiClient.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Handle 401 responses: attempt token refresh or redirect to login
apiClient.interceptors.response.use(
  (response: AxiosResponse) => {
    // Parse version information from response headers
    const versionInfo: VersionResponse = parseVersionResponse(response.headers);
    
    // Log deprecation warnings
    if (versionInfo.isDeprecated) {
      console.warn(
        `API version ${versionInfo.version} is deprecated.`,
        versionInfo.sunsetDate ? `Sunset date: ${versionInfo.sunsetDate}` : '',
        versionInfo.migrationGuide ? `Migration guide: ${versionInfo.migrationGuide}` : '',
      );
    }

    // Validate API response against expected schema in development
    if (import.meta.env.DEV) {
      try {
        validateApiResponse(response);
      } catch (validationError) {
        console.error('API contract validation failed:', validationError);
        // Don't fail the request in development, just log
      }
    }
    
    return response;
  },
  async (error: AxiosError<ApiError>) => {
    const originalRequest = error.config;

    if (error.response?.status === 401 && originalRequest && !originalRequest._retry) {
      originalRequest._retry = true;

      const refreshToken = useAuthStore.getState().refreshToken;
      if (refreshToken) {
        try {
          const { data } = await axios.post(
            `${apiClient.defaults.baseURL}/auth/refresh`,
            { refresh_token: refreshToken },
            { headers: createVersionHeaders(CURRENT_API_VERSION) },
          );
          useAuthStore.getState().setToken(data.access_token, data.refresh_token);
          originalRequest.headers.Authorization = `Bearer ${data.access_token}`;
          return apiClient(originalRequest);
        } catch {
          useAuthStore.getState().logout();
          window.location.href = '/login';
          return Promise.reject(error);
        }
      }

      useAuthStore.getState().logout();
      window.location.href = '/login';
    }

    // Handle API versioning errors specifically
    if (error.response?.status === 400 && error.response.data?.error === 'unsupported_api_version') {
      console.error('Unsupported API version:', error.response.data);
      // Could potentially retry with a different version or show user-friendly error
    }
    
    if (error.response?.status === 410 && error.response.data?.error === 'api_version_sunset') {
      console.error('API version sunset:', error.response.data);
      // Should prompt user to update their client
    }

    // Normalize error
    const normalized: ApiError = error.response?.data ?? {
      error: 'network_error',
      message: error.message || 'An unexpected error occurred',
      status: error.response?.status ?? 0,
    };

    return Promise.reject(normalized);
  },
);

// Extend AxiosRequestConfig to support _retry flag
declare module 'axios' {
  interface InternalAxiosRequestConfig {
    _retry?: boolean;
  }
}

export default apiClient;

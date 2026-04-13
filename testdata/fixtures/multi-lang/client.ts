interface ApiResponse {
    data: string;
}

class ApiClient {
    fetch(): Promise<ApiResponse> {
        return Promise.resolve({ data: "" });
    }
}

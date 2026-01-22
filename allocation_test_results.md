# A/B Allocation Test Results

**Test Date:** 2026-01-22T18:51:44Z

## Test Configuration

- **Total Users:** 100
- **Total Requests:** 500
- **Test Duration:** 968ms
- **Throughput:** 516.38 req/s

## Request Statistics

| Metric | Value |
|--------|-------|
| Total Requests | 500 |
| Successful | 500 |
| Failed | 0 |
| Success Rate | 100.00% |

## Allocation Consistency

| Metric | Value |
|--------|-------|
| Total Users | 100 |
| Consistent Users | 100 |
| Inconsistent Users | 0 |
| **Consistency Rate** | **100.00%** |

### ✅ PASS

All users received consistent payload assignments across multiple requests.
The A/B testing implementation is **deterministic** and working correctly.

## Payload Distribution

This shows how users are distributed across the different payload variants:

| Payload | Users | Percentage |
|---------|-------|------------|
| localization_dummy_3.json | 17 | 17.0% |
| localization_dummy_4.json | 17 | 17.0% |
| localization_example.json | 15 | 15.0% |
| localization_example_2.json | 10 | 10.0% |
| nested_large.json | 21 | 21.0% |
| small_payload.json | 20 | 20.0% |

## Sample User Allocations

First 20 users and their assigned payloads:

| User ID | Payload | Requests | Consistent |
|---------|---------|----------|------------|
| 04b032e2-f9c0-42d3-9072-4c526247f951 | nested_large.json | 5 | ✅ |
| 08682b55-44e7-4d4b-9ffa-1f3d60b51e2b | localization_example.json | 5 | ✅ |
| 086fa144-6841-4bc3-a3b7-194d41484e1d | nested_large.json | 5 | ✅ |
| 09260538-d3ab-44fc-b1ca-3cc77a2e18a7 | nested_large.json | 5 | ✅ |
| 0c36d9a5-d477-4d6a-9425-4454df2bb242 | nested_large.json | 5 | ✅ |
| 1a141357-d052-4fc0-b177-d894d081d57e | localization_dummy_3.json | 5 | ✅ |
| 1f20a9ae-85a6-4d32-b778-89dd62eb452a | localization_dummy_4.json | 5 | ✅ |
| 21fa0985-8ef7-4069-a57e-69113193bb2e | localization_example.json | 5 | ✅ |
| 23857fc1-c5d0-45ed-ac47-cbee53ab81bd | nested_large.json | 5 | ✅ |
| 2476a521-3bf0-4a3b-94de-bce9e2bc4fa1 | small_payload.json | 5 | ✅ |
| 256561dc-3227-482a-a1dc-3a54886376cb | localization_dummy_4.json | 5 | ✅ |
| 2a8baa3c-d927-4e03-acc1-61a99e809591 | localization_dummy_4.json | 5 | ✅ |
| 30b38797-9650-4057-9d09-1bfcb5525f43 | nested_large.json | 5 | ✅ |
| 3134304d-3e54-4f65-afd9-11957c184526 | small_payload.json | 5 | ✅ |
| 3168bc3d-3c86-44b9-81c2-a59bb1756827 | small_payload.json | 5 | ✅ |
| 3589fe6c-5598-44db-a287-49bfcfe9e5d9 | localization_dummy_3.json | 5 | ✅ |
| 35c6c50c-165a-4722-b8f4-8a160801b357 | small_payload.json | 5 | ✅ |
| 368c2e51-e269-40a7-bcdb-8c505a89e1cf | small_payload.json | 5 | ✅ |
| 3cdc5e1f-4a10-4320-abcf-46aa42882a50 | localization_example.json | 5 | ✅ |
| 410d957f-8be3-4a3c-83e1-2975cbbf4d36 | nested_large.json | 5 | ✅ |

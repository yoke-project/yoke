"""A message the wheel carries encodes to the bytes the definitions' Go encodes it to, and decodes back."""

import unittest

from yoke.plugin.v1 import register_pb2

ENCODED = "0a0761637175697265120761637175697265200132047275737442130a116465766963653a696e737472756d656e74"


def request():
    return register_pb2.RegisterRequest(
        plugin="acquire",
        unit="acquire",
        protocol=1,
        language="rust",
        declared=register_pb2.Surface(capabilities=["device:instrument"]),
    )


class Wire(unittest.TestCase):
    def test_a_registration_request_encodes_as_the_go_does_and_decodes_back(self):
        encoded = request().SerializeToString(deterministic=True)
        self.assertEqual(encoded.hex(), ENCODED)
        self.assertEqual(register_pb2.RegisterRequest.FromString(encoded), request())

    def test_the_services_import(self):
        from yoke.plugin.v1 import register_pb2_grpc, session_pb2_grpc

        self.assertTrue(hasattr(register_pb2_grpc, "RegisterStub"))
        self.assertTrue(hasattr(session_pb2_grpc, "SessionStub"))


if __name__ == "__main__":
    unittest.main()
